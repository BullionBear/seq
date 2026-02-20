package xarb

import (
	"math"
	"strings"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
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
	cache         *cache.Cache
	quotingSymbol cpanel.Symbol
	hedgingSymbol cpanel.Symbol

	// Account IDs for trading
	quotingAccount cpanel.Account
	hedgingAccount cpanel.Account

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
			// Market data
			event.TopicEventDepthUpdate,
			// Execution data
			event.TopicEventExecution,
			// Reconciliation data
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
			event.TopicEventOrderError,
			event.TopicEventOrderRiskInvalid,
			event.TopicEventOrderNew,
			event.TopicEventOrderAccepted,
		}),
		cache:         cache,
		quotingSymbol: cpanel.Symbol{},
		hedgingSymbol: cpanel.Symbol{},

		quotingAccount: cpanel.Account{},
		hedgingAccount: cpanel.Account{},

		Side:              common.SideUnknown,
		ProfitBps:         0.0,
		Qty:               0.0,
		PriceToleranceBps: 0.0000,

		quotingClientOrderID: 0,
		hedgingClientOrderID: 0,
		unhedgedAvailable:    0.0,
		unhedgedLocked:       0.0,
		quotingCount:         0,
		hedgingCount:         0,
	}
}

// OnInit initializes the strategy with configuration.
func (x *XArb) OnInit(config map[string]any) {
	// Get strategy-specific config from StrategyBase
	var xarbConfig XArbConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &xarbConfig,
		TagName: "yaml", // Use yaml tags for mapping
	})
	if err != nil {
		x.Log().Panic().Msg("failed to create decoder")
		return
	}

	err = decoder.Decode(config)
	if err != nil {
		x.Log().Panic().Msg("failed to decode config")
		return
	}

	// Resolve trading symbols
	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.QuotingSymbolUniversalTicker)
	if err != nil {
		x.Log().Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.HedgingSymbolUniversalTicker)
	if err != nil {
		x.Log().Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
	x.Log().Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.UniversalTicker, quotingSymbol.ID)
	x.Log().Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.UniversalTicker, hedgingSymbol.ID)

	// Resolve algo parameters
	switch strings.ToLower(xarbConfig.Side) {
	case "buy", "b":
		x.Side = common.SideBuy
	case "sell", "s":
		x.Side = common.SideSell
	default:
		x.Log().Panic().Str("side", xarbConfig.Side).Msg("invalid side")
	}
	x.ProfitBps = xarbConfig.ProfitBps
	x.Qty = xarbConfig.Qty
	x.PriceToleranceBps = xarbConfig.PriceToleranceBps
	x.Log().Info().Str("side", x.Side.String()).Float64("profitBps", x.ProfitBps).Float64("qty", x.Qty).Float64("priceToleranceBps", x.PriceToleranceBps).Msg("XArb config")
	// Resolve trading accounts
	if xarbConfig.QuotingAccount != "" {
		quotingAccount := x.GetCatalog().GetAccountByName(xarbConfig.QuotingAccount)
		if quotingAccount != nil {
			x.quotingAccount = *quotingAccount
			x.Log().Info().Msgf("Quoting account: %s(%d)", quotingAccount.Name, quotingAccount.ID)
		} else {
			x.Log().Warn().Str("account", xarbConfig.QuotingAccount).Msg("quoting account not found")
		}
	}

	if xarbConfig.HedgingAccount != "" {
		hedgingAccount := x.GetCatalog().GetAccountByName(xarbConfig.HedgingAccount)
		if hedgingAccount != nil {
			x.hedgingAccount = *hedgingAccount
			x.Log().Info().Msgf("Hedging account: %s(%d)", hedgingAccount.Name, hedgingAccount.ID)
		} else {
			x.Log().Warn().Str("account", xarbConfig.HedgingAccount).Msg("hedging account not found")
		}
	}

}

// OnStart is called when the strategy starts.
// Note: Data subscriptions are now handled by the config - no manual Subscribe/Connect needed.
func (x *XArb) OnStart() {
	x.Log().Info().Msgf("XArb strategy started for symbols: %s, %s",
		x.quotingSymbol.UniversalTicker, x.hedgingSymbol.UniversalTicker)
}

// OnStop is called when the strategy stops.
func (x *XArb) OnStop() {
	x.Log().Info().Msg("XArb strategy stopped")
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

// OnDepthUpdate processes depth updates.
// Note: Snapshot requests are now handled automatically by DataEngine.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	if update.SymbolID != x.hedgingSymbol.ID && update.SymbolID != x.quotingSymbol.ID {
		return
	}
	x.Log().Debug().Int("symbolID", update.SymbolID).Int("depthID", update.DepthID).Msg("DepthUpdate")
	if x.cache.IsSymbolReady(update.SymbolID) && update.SymbolID == x.hedgingSymbol.ID {
		x.hedgingCount += 1
	}
	if x.cache.IsSymbolReady(update.SymbolID) && update.SymbolID == x.quotingSymbol.ID {
		x.quotingCount += 1
	}
	if x.hedgingCount == 0 || x.quotingCount == 0 {
		x.Log().Info().Msg("XArb strategy is not ready")
		return
	}
	if (x.quotingCount+x.hedgingCount)%100 == 0 {
		x.Log().Info().Int("quotingCount", x.quotingCount).Int("hedgingCount", x.hedgingCount).Msg("XArb strategy is ready")
	}
	switch x.Side {
	case common.SideBuy:
		refPrice, _, ok := x.cache.GetBestBid(x.hedgingSymbol.ID)
		if !ok {
			x.Log().Error().Msg("failed to get best bid")
			return
		}
		if x.quotingClientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.quotingAccount.ID, x.quotingClientOrderID)
			if ok {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order already submitted")
				return
			}
			if order.OrderStatus.IsTerminal() {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order already terminal")
				x.quotingClientOrderID = 0
				return
			}
			if !order.OrderStatus.Cancellable() {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order not cancellable")
				return
			}
			if math.Abs((order.Price-refPrice)/refPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Float64("priceToleranceBps", x.PriceToleranceBps).Msg("Quote order price tolerance exceeded")
				x.CancelOrder(x.quotingClientOrderID, x.quotingAccount.ID)
				return
			}
		}
		priceDecimal := math.Pow10(x.quotingSymbol.PricePrecision)
		buyPrice := math.Floor(refPrice*(1.0-x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		buyQty := x.Qty
		x.Log().Info().Float64("buyPrice", buyPrice).Float64("buyQty", buyQty).Str("side", "buy").Msg("Submit quote order")
		x.quotingClientOrderID = x.SubmitOrder(x.quotingAccount.ID, x.quotingSymbol.ID, common.SideBuy, common.OrderTypeLimit, common.TimeInForcePO, buyPrice, buyQty)
		x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order submitted")
	case common.SideSell:
		refPrice, _, ok := x.cache.GetBestAsk(x.quotingSymbol.ID)
		if !ok {
			x.Log().Error().Msg("failed to get best ask")
			return
		}
		if x.quotingClientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.quotingAccount.ID, x.quotingClientOrderID)
			if ok {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order already submitted")
				return
			}
			if order.OrderStatus.IsTerminal() {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order already terminal")
				x.quotingClientOrderID = 0
				return
			}
			if !order.OrderStatus.Cancellable() {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order not cancellable")
				return
			}
			if math.Abs((order.Price-refPrice)/refPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Float64("priceToleranceBps", x.PriceToleranceBps).Msg("Quote order price tolerance exceeded")
				x.CancelOrder(x.quotingClientOrderID, x.quotingAccount.ID)
				return
			}
		}
		priceDecimal := math.Pow10(x.quotingSymbol.PricePrecision)
		sellPrice := math.Ceil(refPrice*(1.0+x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		sellQty := x.Qty
		x.Log().Info().Float64("sellPrice", sellPrice).Float64("sellQty", sellQty).Str("side", "sell").Msg("Submit hedge order")
		x.quotingClientOrderID = x.SubmitOrder(x.quotingAccount.ID, x.quotingSymbol.ID, common.SideSell, common.OrderTypeLimit, common.TimeInForcePO, sellPrice, sellQty)
		x.Log().Info().Int("clientOrderID", x.quotingClientOrderID).Msg("Quote order submitted")
	}
}

// OnExecution processes execution (fill) events.
func (x *XArb) OnExecution(exec event.Execution) {
	if exec.SymbolID == x.quotingSymbol.ID && exec.Side == common.SideBuy {
		x.unhedgedAvailable += exec.FilledQty
	} else if exec.SymbolID == x.quotingSymbol.ID && exec.Side == common.SideSell {
		x.unhedgedAvailable -= exec.FilledQty
	} else if exec.SymbolID == x.hedgingSymbol.ID && exec.Side == common.SideBuy {
		x.unhedgedLocked += exec.FilledQty
	} else if exec.SymbolID == x.hedgingSymbol.ID && exec.Side == common.SideSell {
		x.unhedgedLocked -= exec.FilledQty
	} else {
		x.Log().Error().Int("symbolID", exec.SymbolID).Str("side", exec.Side.String()).Msg("Invalid execution")
		return
	}
	if math.Abs(x.unhedgedAvailable) < 1e-6 {
		x.Log().Info().Msg("No unhedged available")
		return
	}
	if x.unhedgedAvailable < 0 {
		qty := -x.unhedgedAvailable
		x.unhedgedAvailable = 0
		x.unhedgedLocked += qty
		x.SubmitOrder(x.hedgingAccount.ID, x.hedgingSymbol.ID, common.SideBuy, common.OrderTypeMarket, common.TimeInForceIOC, 0, qty)
	} else if x.unhedgedAvailable > 0 {
		qty := x.unhedgedAvailable
		x.unhedgedAvailable = 0
		x.unhedgedLocked += qty
		x.SubmitOrder(x.quotingAccount.ID, x.quotingSymbol.ID, common.SideSell, common.OrderTypeMarket, common.TimeInForceIOC, 0, qty)
	} else {
		x.Log().Error().Msg("Invalid unhedged available")
		return
	}
}

func (x *XArb) OnOrderCanceled(orderCanceled event.OrderCanceled) {
	if orderCanceled.AccountID == x.quotingAccount.ID {
		x.quotingClientOrderID = 0
	} else {
		x.Log().Error().Int("accountID", orderCanceled.AccountID).Msg("Invalid account ID")
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
