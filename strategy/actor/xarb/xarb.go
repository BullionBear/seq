package xarb

import (
	"math"
	"strings"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/mitchellh/mapstructure"
)

func init() {
	strategy.Register("xarb", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewXArb(cat, bus, c)
	})
}

// Ensure XArb implements the Actor interface
var _ actor.Actor = (*XArb)(nil)

// XArb is a cross-exchange arbitrage strategy.
type XArb struct {
	strategy.StrategyActorBase
	cache *cache.Cache

	// Resolved symbols
	quotingSymbolID       int
	hedgingSymbolID       int
	quotingSymbolTicker   string
	hedgingSymbolTicker   string
	quotingPricePrecision int
	hedgingPricePrecision int

	// Wallet-derived account IDs for order routing
	quotingAccountID int
	hedgingAccountID int

	// Algo parameters
	Side              common.Side
	ProfitBps         float64
	Qty               float64
	PriceToleranceBps float64

	// Algo variables
	quotingClientOrderID int
	hedgingClientOrderID int
	quotingCount         int
	hedgingCount         int
	unhedgedAvailable    float64
	unhedgedLocked       float64
}

// NewXArb creates a new XArb strategy.
func NewXArb(catalog *catalog.Catalog, msgbus *msgbus.MsgBus, cache *cache.Cache) *XArb {
	return &XArb{
		StrategyActorBase: strategy.NewStrategyActorBase("xarb", catalog, msgbus, []event.Topic{
			event.TopicEventDepthUpdate,
			event.TopicEventExecution,
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
			event.TopicEventOrderFilled,
			event.TopicEventOrderError,
			event.TopicEventOrderRiskInvalid,
			event.TopicEventOrderNew,
			event.TopicEventOrderAccepted,
		}),
		cache: cache,
	}
}

// OnInit initializes the strategy with configuration.
func (x *XArb) OnInit(config map[string]any) {
	var cfg XArbConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		x.Log().Panic().Msg("failed to create decoder")
		return
	}
	if err = decoder.Decode(config); err != nil {
		x.Log().Panic().Msg("failed to decode config")
		return
	}

	// Resolve trading symbols
	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(cfg.QuotingSymbolUniversalTicker)
	if err != nil {
		x.Log().Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(cfg.HedgingSymbolUniversalTicker)
	if err != nil {
		x.Log().Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	x.quotingSymbolID = quotingSymbol.ID
	x.hedgingSymbolID = hedgingSymbol.ID
	x.quotingSymbolTicker = quotingSymbol.UniversalTicker
	x.hedgingSymbolTicker = hedgingSymbol.UniversalTicker
	x.quotingPricePrecision = quotingSymbol.PricePrecision
	x.hedgingPricePrecision = hedgingSymbol.PricePrecision
	x.Log().Info().
		Str("quotingSymbol", x.quotingSymbolTicker).Int("quotingSymbolID", x.quotingSymbolID).
		Str("hedgingSymbol", x.hedgingSymbolTicker).Int("hedgingSymbolID", x.hedgingSymbolID).
		Msg("Resolved symbols")

	// Resolve algo parameters
	switch strings.ToLower(cfg.Side) {
	case "buy", "b":
		x.Side = common.SideBuy
	case "sell", "s":
		x.Side = common.SideSell
	default:
		x.Log().Panic().Str("side", cfg.Side).Msg("invalid side")
	}
	x.ProfitBps = cfg.ProfitBps
	x.Qty = cfg.Qty
	x.PriceToleranceBps = cfg.PriceToleranceBps
	x.Log().Info().Str("side", x.Side.String()).
		Float64("profitBps", x.ProfitBps).
		Float64("qty", x.Qty).
		Float64("priceToleranceBps", x.PriceToleranceBps).
		Msg("Algo parameters")

	// Resolve trading wallets
	if cfg.QuotingWallet != "" {
		wallet, err := x.GetCatalog().GetWalletByName(cfg.QuotingWallet)
		if err != nil {
			x.Log().Panic().Err(err).Str("wallet", cfg.QuotingWallet).Msg("quoting wallet not found")
			return
		}
		x.quotingAccountID = wallet.AcctID
		x.Log().Info().Str("wallet", wallet.Name).Int("accountID", wallet.AcctID).
			Str("walletType", wallet.WalletType.String()).Msg("Quoting wallet resolved")
	}

	if cfg.HedgingWallet != "" {
		wallet, err := x.GetCatalog().GetWalletByName(cfg.HedgingWallet)
		if err != nil {
			x.Log().Panic().Err(err).Str("wallet", cfg.HedgingWallet).Msg("hedging wallet not found")
			return
		}
		x.hedgingAccountID = wallet.AcctID
		x.Log().Info().Str("wallet", wallet.Name).Int("accountID", wallet.AcctID).
			Str("walletType", wallet.WalletType.String()).Msg("Hedging wallet resolved")
	}
}

func (x *XArb) OnStart() {
	x.Log().Info().
		Str("quotingSymbol", x.quotingSymbolTicker).
		Str("hedgingSymbol", x.hedgingSymbolTicker).
		Msg("XArb strategy started")
}

// OnStop is called when the strategy stops.
func (x *XArb) OnStop() {
	x.Log().Info().Msg("XArb strategy stopped")
	if x.quotingClientOrderID != 0 {
		x.CancelOrder(x.quotingClientOrderID, x.quotingAccountID)
	}
}

// Handle overrides StrategyBase.Handle to dispatch events to XArb's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (x *XArb) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		x.OnDepthUpdate(update)
	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec := event.NewExecutionFromBytes(buf)
		x.OnExecution(exec)
	case event.TopicEventOrderCanceled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderCanceled := event.NewOrderCanceledFromBytes(buf)
		x.OnOrderCanceled(orderCanceled)
	case event.TopicEventOrderRejected:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderRejected := event.NewOrderRejectedFromBytes(buf)
		x.OnOrderRejected(orderRejected)
	case event.TopicEventOrderFilled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderFilled := event.NewOrderFilledFromBytes(buf)
		x.OnOrderFilled(orderFilled)
	case event.TopicEventOrderError:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderError := event.NewOrderErrorFromBytes(buf)
		x.OnOrderError(orderError)
	case event.TopicEventOrderRiskInvalid:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderRiskInvalid := event.NewOrderRiskInvalidFromBytes(buf)
		x.OnOrderRiskInvalid(orderRiskInvalid)
	case event.TopicEventOrderNew:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderNew := event.NewOrderNewFromBytes(buf)
		x.OnOrderNew(orderNew)
	case event.TopicEventOrderAccepted:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderAccepted := event.NewOrderAcceptedFromBytes(buf)
		x.OnOrderAccepted(orderAccepted)
	}
}

func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	if update.SymbolID != x.hedgingSymbolID && update.SymbolID != x.quotingSymbolID {
		return
	}
	x.Log().Debug().Int("symbolID", update.SymbolID).Int("depthID", update.DepthID).Msg("DepthUpdate")
	if x.cache.IsSymbolReady(update.SymbolID) && update.SymbolID == x.hedgingSymbolID {
		x.hedgingCount += 1
	}
	if x.cache.IsSymbolReady(update.SymbolID) && update.SymbolID == x.quotingSymbolID {
		x.quotingCount += 1
	}
	if x.hedgingCount == 0 || x.quotingCount == 0 {
		x.Log().Info().Msg("XArb strategy is not ready")
		return
	}
	if (x.quotingCount+x.hedgingCount)%100 == 0 {
		quoteBid, _, _ := x.cache.GetBestBid(x.quotingSymbolID)
		quoteAsk, _, _ := x.cache.GetBestAsk(x.quotingSymbolID)
		hedgeBid, _, _ := x.cache.GetBestBid(x.hedgingSymbolID)
		hedgeAsk, _, _ := x.cache.GetBestAsk(x.hedgingSymbolID)
		x.Log().Info().
			Int("quotingCount", x.quotingCount).Int("hedgingCount", x.hedgingCount).
			Float64("quoteBid", quoteBid).Float64("quoteAsk", quoteAsk).
			Float64("hedgeBid", hedgeBid).Float64("hedgeAsk", hedgeAsk).
			Msg("XArb strategy is ready")
	}
	switch x.Side {
	case common.SideBuy:
		refPrice, _, ok := x.cache.GetBestBid(x.hedgingSymbolID)
		if !ok {
			x.Log().Error().Msg("failed to get best bid")
			return
		}
		priceDecimal := math.Pow10(x.quotingPricePrecision)
		buyPrice := math.Floor(refPrice*(1.0-x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		if x.quotingClientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.quotingAccountID, x.quotingClientOrderID)
			if !ok {
				x.quotingClientOrderID = 0
			} else if order.OrderStatus.IsTerminal() {
				x.quotingClientOrderID = 0
				return
			} else if !order.OrderStatus.Cancellable() {
				return
			} else if math.Abs((order.Price-buyPrice)/buyPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).
					Float64("orderPrice", order.Price).Float64("targetPrice", buyPrice).
					Float64("bpsGap", math.Abs((order.Price-buyPrice)/buyPrice)*10000.0).
					Msg("Quote order price tolerance exceeded, cancelling")
				x.CancelOrder(x.quotingClientOrderID, x.quotingAccountID)
				return
			} else {
				return
			}
		}
		buyQty := x.Qty
		x.Log().Info().Float64("buyPrice", buyPrice).Float64("refPrice", refPrice).Float64("buyQty", buyQty).Str("side", "buy").Msg("Submit quote order")
		x.quotingClientOrderID = x.SubmitOrder(x.quotingAccountID, x.quotingSymbolID, common.SideBuy, common.OrderTypeLimit, common.TimeInForcePO, buyPrice, buyQty)
		x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order submitted")
	case common.SideSell:
		refPrice, _, ok := x.cache.GetBestAsk(x.quotingSymbolID)
		if !ok {
			x.Log().Error().Msg("failed to get best ask")
			return
		}
		priceDecimal := math.Pow10(x.quotingPricePrecision)
		sellPrice := math.Ceil(refPrice*(1.0+x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		if x.quotingClientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.quotingAccountID, x.quotingClientOrderID)
			if !ok {
				x.quotingClientOrderID = 0
			} else if order.OrderStatus.IsTerminal() {
				x.quotingClientOrderID = 0
				return
			} else if !order.OrderStatus.Cancellable() {
				return
			} else if math.Abs((order.Price-sellPrice)/sellPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).
					Float64("orderPrice", order.Price).Float64("targetPrice", sellPrice).
					Float64("bpsGap", math.Abs((order.Price-sellPrice)/sellPrice)*10000.0).
					Msg("Quote order price tolerance exceeded, cancelling")
				x.CancelOrder(x.quotingClientOrderID, x.quotingAccountID)
				return
			} else {
				return
			}
		}
		sellQty := x.Qty
		x.Log().Info().Float64("sellPrice", sellPrice).Float64("refPrice", refPrice).Float64("sellQty", sellQty).Str("side", "sell").Msg("Submit quote order")
		x.quotingClientOrderID = x.SubmitOrder(x.quotingAccountID, x.quotingSymbolID, common.SideSell, common.OrderTypeLimit, common.TimeInForcePO, sellPrice, sellQty)
		x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order submitted")
	}
}

func (x *XArb) OnExecution(exec event.Execution) {
	if exec.SymbolID == x.quotingSymbolID && exec.Side == common.SideBuy {
		x.unhedgedAvailable += exec.FilledQty
	} else if exec.SymbolID == x.quotingSymbolID && exec.Side == common.SideSell {
		x.unhedgedAvailable -= exec.FilledQty
	} else if exec.SymbolID == x.hedgingSymbolID && exec.Side == common.SideBuy {
		x.unhedgedLocked += exec.FilledQty
	} else if exec.SymbolID == x.hedgingSymbolID && exec.Side == common.SideSell {
		x.unhedgedLocked -= exec.FilledQty
	} else {
		return
	}
	if math.Abs(x.unhedgedAvailable) < x.Qty*(1.0-1e-6) {
		x.Log().Info().Float64("unhedgedAvailable", x.unhedgedAvailable).Float64("qty", x.Qty).Msg("Unhedged below threshold, waiting")
		return
	}
	if x.unhedgedAvailable < 0 {
		qty := -x.unhedgedAvailable
		x.unhedgedAvailable = 0
		x.unhedgedLocked += qty
		x.SubmitOrder(x.hedgingAccountID, x.hedgingSymbolID, common.SideBuy, common.OrderTypeMarket, common.TimeInForceIOC, 0, qty)
	} else if x.unhedgedAvailable > 0 {
		qty := x.unhedgedAvailable
		x.unhedgedAvailable = 0
		x.unhedgedLocked += qty
		x.SubmitOrder(x.hedgingAccountID, x.hedgingSymbolID, common.SideSell, common.OrderTypeMarket, common.TimeInForceIOC, 0, qty)
	} else {
		x.Log().Error().Msg("Invalid unhedged available")
		return
	}
}

func (x *XArb) OnOrderCanceled(orderCanceled event.OrderCanceled) {
	switch orderCanceled.ClientOrderID {
	case x.quotingClientOrderID:
		x.Log().Info().Int("clientOrderID", orderCanceled.ClientOrderID).Msg("Quote order cancelled")
		x.quotingClientOrderID = 0
	case x.hedgingClientOrderID:
		x.Log().Info().Int("clientOrderID", orderCanceled.ClientOrderID).Msg("Hedge order cancelled")
		x.hedgingClientOrderID = 0
	default:
		return
	}
}

func (x *XArb) OnOrderFilled(orderFilled event.OrderFilled) {
	switch orderFilled.ClientOrderID {
	case x.quotingClientOrderID:
		x.quotingClientOrderID = 0
		x.Log().Info().Int("Quoting clientOrderID", orderFilled.ClientOrderID).Float64("executedQty", orderFilled.ExecutedQty).Msg("Order filled")
	case x.hedgingClientOrderID:
		x.hedgingClientOrderID = 0
		x.Log().Info().Int("Hedging clientOrderID", orderFilled.ClientOrderID).Float64("executedQty", orderFilled.ExecutedQty).Msg("Order filled")
	default:
		// Ignore other client order IDs
		return
	}
}

func (x *XArb) OnOrderRejected(orderRejected event.OrderRejected) {
	switch orderRejected.ClientOrderID {
	case x.quotingClientOrderID:
		x.quotingClientOrderID = 0
		x.Log().Info().Int("Quoting clientOrderID", orderRejected.ClientOrderID).Str("msg", orderRejected.Msg).Msg("Order rejected")
	case x.hedgingClientOrderID:
		x.hedgingClientOrderID = 0
		x.Log().Info().Int("Hedging clientOrderID", orderRejected.ClientOrderID).Str("msg", orderRejected.Msg).Msg("Order rejected")
	default:
		// Ignore other client order IDs
		return
	}
}

func (x *XArb) OnOrderError(orderError event.OrderError) {
	x.Log().Error().Int("clientOrderID", orderError.ClientOrderID).Int("orderID", orderError.OrderID).Int("accountID", orderError.AccountID).Int("errorCode", orderError.ErrorCode).Msg("Order error")
}

func (x *XArb) OnOrderRiskInvalid(orderRiskInvalid event.OrderRiskInvalid) {
	switch orderRiskInvalid.ClientOrderID {
	case x.quotingClientOrderID:
		x.quotingClientOrderID = 0
		x.Log().Info().Int("Quoting clientOrderID", orderRiskInvalid.ClientOrderID).Msg("Order risk invalid")
	case x.hedgingClientOrderID:
		x.hedgingClientOrderID = 0
		x.Log().Info().Int("Hedging clientOrderID", orderRiskInvalid.ClientOrderID).Msg("Order risk invalid")
	default:
		// Ignore other client order IDs
		return
	}
}

func (x *XArb) OnOrderNew(orderNew event.OrderNew) {
	switch orderNew.ClientOrderID {
	case x.quotingClientOrderID:
		x.quotingClientOrderID = orderNew.ClientOrderID
		x.Log().Info().Int("Quoting clientOrderID", orderNew.ClientOrderID).Msg("Order new")
	case x.hedgingClientOrderID:
		x.hedgingClientOrderID = orderNew.ClientOrderID
		x.Log().Info().Int("Hedging clientOrderID", orderNew.ClientOrderID).Msg("Order new")
	default:
		// Ignore other client order IDs
		return
	}
}

func (x *XArb) OnOrderAccepted(orderAccepted event.OrderAccepted) {
	switch orderAccepted.ClientOrderID {
	case x.quotingClientOrderID:
		x.quotingClientOrderID = orderAccepted.ClientOrderID
		x.Log().Info().Int("Quoting clientOrderID", orderAccepted.ClientOrderID).Msg("Order accepted")
	case x.hedgingClientOrderID:
		x.hedgingClientOrderID = orderAccepted.ClientOrderID
		x.Log().Info().Int("Hedging clientOrderID", orderAccepted.ClientOrderID).Msg("Order accepted")
	default:
		// Ignore other client order IDs
		return
	}
}
