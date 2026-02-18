package xarb

import (
	"math"
	"sync"
	"time"

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

	// Count
	quotingCount int
	hedgingCount int
	// Once
	once sync.Once
}

// NewXArb creates a new XArb strategy.
func NewXArb(catalog *catalog.Catalog, msgbus *msgbus.MsgBus, cache *cache.Cache) *XArb {
	return &XArb{
		StrategyActorBase: strategy.NewStrategyActorBase("xarb", catalog, msgbus, []event.Topic{
			// Market data
			event.TopicEventDepthSnapshot,
			event.TopicEventDepthUpdate,
			// Execution data
			event.TopicEventOrderPartialFill,
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

		quotingCount: 0,
		hedgingCount: 0,
		once:         sync.Once{},
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
	case event.TopicEventOrderPartialFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		partialFill := event.NewOrderPartiallyFilledFromBytes(buf)
		x.OnPartialFill(partialFill)
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
	if x.quotingCount%50 == 0 {
		x.Log().Info().Msgf("Quoting count: %d", x.quotingCount)
		price, ok := x.cache.GetMidPrice(x.quotingSymbol.ID)
		if !ok {
			x.Log().Error().Msg("failed to get mid price")
			return
		}
		x.once.Do(func() {
			buyPrice := price * 1.005
			pricePrecision := x.quotingSymbol.PricePrecision
			buyPrice = math.Ceil(buyPrice*math.Pow10(pricePrecision)) / math.Pow10(pricePrecision)
			buyQty := 2.0
			x.Log().Info().Msgf("Submit order: %f@%f", buyPrice, buyQty)
			x.SubmitOrder(int(time.Now().UnixMilli()), x.quotingAccount.ID, x.quotingSymbol.ID, common.SideBuy, common.OrderTypeLimit, common.TimeInForceGTC, buyPrice, buyQty)
		})
	}
	if x.hedgingCount%50 == 0 {
		x.Log().Info().Msgf("Hedging count: %d", x.hedgingCount)
	}
}

// OnRespDepthSnapshot processes the response to a depth snapshot request.
func (x *XArb) OnRespDepthSnapshot(snapshot event.RespDepthSnapshot) {
	symbolID := snapshot.SymbolID
	x.Log().Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asks", len(snapshot.Asks)).
		Int("bids", len(snapshot.Bids)).
		Msg("RespDepthSnapshot received")
}

// OnExecution processes execution (fill) events.
func (x *XArb) OnExecution(exec event.Execution) {}

func (x *XArb) OnPartialFill(partialFill event.OrderPartiallyFilled) {}

func (x *XArb) OnOrderCanceled(orderCanceled event.OrderCanceled) {}

func (x *XArb) OnOrderRejected(orderRejected event.OrderRejected) {
	x.Log().Error().Int("clientOrderID", orderRejected.ClientOrderID).Int("orderID", orderRejected.OrderID).Int("errorCode", orderRejected.ErrorCode).Msg("Order rejected")
}

func (x *XArb) OnOrderError(orderError event.OrderError) {}

func (x *XArb) OnOrderRiskInvalid(orderRiskInvalid event.OrderRiskInvalid) {}

func (x *XArb) OnOrderNew(orderNew event.OrderNew) {
	x.Log().Info().Int("clientOrderID", orderNew.ClientOrderID).Int("orderID", orderNew.OrderID).Int("accountID", orderNew.AccountID).Uint64("createdAt", orderNew.CreatedAt).Msg("Order new")
}

func (x *XArb) OnOrderAccepted(orderAccepted event.OrderAccepted) {
	x.Log().Info().Int("clientOrderID", orderAccepted.ClientOrderID).Int("orderID", orderAccepted.OrderID).Uint64("createdAt", orderAccepted.CreatedAt).Msg("Order accepted")
}
