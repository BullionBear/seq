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
	clientOrderID     int
	quotingCount      int
	hedgingCount      int
	unhedgedAvailable float64
	unhedgedLocked    float64
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

		clientOrderID:     0,
		unhedgedAvailable: 0.0,
		unhedgedLocked:    0.0,
		quotingCount:      0,
		hedgingCount:      0,
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
	if update.SymbolID != x.hedgingSymbol.ID || update.SymbolID != x.quotingSymbol.ID {
		return
	}
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
	switch x.Side {
	case common.SideBuy:
		refPrice, _, ok := x.cache.GetBestBid(x.hedgingSymbol.ID)
		if !ok {
			x.Log().Error().Msg("failed to get best bid")
			return
		}
		if x.clientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.quotingAccount.ID, x.clientOrderID)
			if ok {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Quote order already submitted")
				return
			}
			if order.OrderStatus.IsTerminal() {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Quote order already terminal")
				x.clientOrderID = 0
				return
			}
			if !order.OrderStatus.Cancellable() {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Quote order not cancellable")
				return
			}
			if math.Abs((order.Price-refPrice)/refPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Float64("priceToleranceBps", x.PriceToleranceBps).Msg("Quote order price tolerance exceeded")
				x.CancelOrder(x.clientOrderID, x.quotingAccount.ID)
				return
			}
		}
		priceDecimal := math.Pow10(x.quotingSymbol.PricePrecision)
		buyPrice := math.Ceil(refPrice*(1.0+x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		buyQty := x.Qty
		x.Log().Info().Float64("buyPrice", buyPrice).Float64("buyQty", buyQty).Str("side", "buy").Msg("Submit quote order")
		x.clientOrderID = x.SubmitOrder(x.quotingAccount.ID, x.quotingSymbol.ID, common.SideBuy, common.OrderTypeLimit, common.TimeInForcePO, buyPrice, buyQty)
		x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Quote order submitted")
	case common.SideSell:
		refPrice, _, ok := x.cache.GetBestAsk(x.quotingSymbol.ID)
		if !ok {
			x.Log().Error().Msg("failed to get best ask")
			return
		}
		if x.clientOrderID != 0 {
			order, ok := x.cache.GetOpenOrder(x.hedgingAccount.ID, x.clientOrderID)
			if ok {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Hedge order already submitted")
				return
			}
			if order.OrderStatus.IsTerminal() {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Hedge order already terminal")
				x.clientOrderID = 0
				return
			}
			if !order.OrderStatus.Cancellable() {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Hedge order not cancellable")
				return
			}
			if math.Abs((order.Price-refPrice)/refPrice) > x.PriceToleranceBps/10000.0 {
				x.Log().Info().Int("clientOrderID", x.clientOrderID).Float64("priceToleranceBps", x.PriceToleranceBps).Msg("Hedge order price tolerance exceeded")
				x.CancelOrder(x.clientOrderID, x.hedgingAccount.ID)
				return
			}
		}
		priceDecimal := math.Pow10(x.quotingSymbol.PricePrecision)
		sellPrice := math.Floor(refPrice*(1.0-x.ProfitBps/10000.0)*priceDecimal) / priceDecimal
		sellQty := x.Qty
		x.Log().Info().Float64("sellPrice", sellPrice).Float64("sellQty", sellQty).Str("side", "sell").Msg("Submit hedge order")
		x.clientOrderID = x.SubmitOrder(x.hedgingAccount.ID, x.hedgingSymbol.ID, common.SideSell, common.OrderTypeLimit, common.TimeInForcePO, sellPrice, sellQty)
		x.Log().Info().Int("clientOrderID", x.clientOrderID).Msg("Hedge order submitted")
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
		x.unhedgedAvailable += x.unhedgedAvailable
		x.unhedgedLocked -= x.unhedgedAvailable
		x.SubmitOrder(x.hedgingAccount.ID, x.hedgingSymbol.ID, common.SideBuy, common.OrderTypeMarket, common.TimeInForceIOC, 0, -x.unhedgedAvailable)
	} else if x.unhedgedAvailable > 0 {
		x.unhedgedAvailable -= x.unhedgedAvailable
		x.unhedgedLocked += x.unhedgedAvailable
		x.SubmitOrder(x.quotingAccount.ID, x.quotingSymbol.ID, common.SideSell, common.OrderTypeMarket, common.TimeInForceIOC, 0, x.unhedgedAvailable)
	} else {
		x.Log().Error().Msg("Invalid unhedged available")
		return
	}
}

func (x *XArb) OnOrderCanceled(orderCanceled event.OrderCanceled) {
	if orderCanceled.AccountID == x.quotingAccount.ID {
		x.clientOrderID = 0
	} else {
		x.Log().Error().Int("accountID", orderCanceled.AccountID).Msg("Invalid account ID")
		return
	}
}

func (x *XArb) OnOrderRejected(orderRejected event.OrderRejected) {
	if orderRejected.ClientOrderID == x.clientOrderID {
		x.clientOrderID = 0
	} else {
		x.Log().Error().Int("clientOrderID", orderRejected.ClientOrderID).Msg("Invalid client order ID")
		return
	}
}

func (x *XArb) OnOrderError(orderError event.OrderError) {
	x.Log().Error().Int("clientOrderID", orderError.ClientOrderID).Int("orderID", orderError.OrderID).Int("accountID", orderError.AccountID).Int("errorCode", orderError.ErrorCode).Msg("Order error")
}

func (x *XArb) OnOrderRiskInvalid(orderRiskInvalid event.OrderRiskInvalid) {
	if orderRiskInvalid.ClientOrderID == x.clientOrderID {
		x.clientOrderID = 0
	} else {
		x.Log().Error().Int("clientOrderID", orderRiskInvalid.ClientOrderID).Msg("Invalid client order ID")
		return
	}
}

func (x *XArb) OnOrderNew(orderNew event.OrderNew) {
	x.Log().Info().Int("clientOrderID", orderNew.ClientOrderID).Int("orderID", orderNew.OrderID).Int("accountID", orderNew.AccountID).Int("symbolID", orderNew.SymbolID).Float64("price", orderNew.Price).Float64("quantity", orderNew.Quantity).Uint64("createdAt", orderNew.CreatedAt).Msg("Order new")
}

func (x *XArb) OnOrderAccepted(orderAccepted event.OrderAccepted) {
	x.Log().Info().Int("clientOrderID", orderAccepted.ClientOrderID).Int("orderID", orderAccepted.OrderID).Int("accountID", orderAccepted.AccountID).Uint64("createdAt", orderAccepted.CreatedAt).Msg("Order accepted")
}
