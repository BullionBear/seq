package xarb

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	strategy.Register("xarb", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewXArb(cat, bus, c)
	})
}

func log() *zerolog.Logger { l := logger.Get(); return &l }

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
}

// NewXArb creates a new XArb strategy.
func NewXArb(catalog *catalog.Catalog, msgbus *msgbus.MsgBus, cache *cache.Cache) *XArb {
	return &XArb{
		StrategyActorBase: strategy.NewStrategyActorBase("xarb", catalog, msgbus, []event.Topic{
			// Market data
			event.TopicEventDepthSnapshot,
			event.TopicEventDepthUpdate,
			// Execution data
			event.TopicEventPartialFill,
			event.TopicEventFill,
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
		log().Panic().Msg("failed to create decoder")
		return
	}

	err = decoder.Decode(config)
	if err != nil {
		log().Panic().Msg("failed to decode config")
		return
	}

	// Resolve trading symbols
	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.QuotingSymbolUniversalTicker)
	if err != nil {
		log().Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.HedgingSymbolUniversalTicker)
	if err != nil {
		log().Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
	log().Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.UniversalTicker, quotingSymbol.ID)
	log().Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.UniversalTicker, hedgingSymbol.ID)

	// Resolve trading accounts
	if xarbConfig.QuotingAccount != "" {
		quotingAccount := x.GetCatalog().GetAccountByName(xarbConfig.QuotingAccount)
		if quotingAccount != nil {
			x.quotingAccount = *quotingAccount
			log().Info().Msgf("Quoting account: %s(%d)", quotingAccount.Name, quotingAccount.ID)
		} else {
			log().Warn().Str("account", xarbConfig.QuotingAccount).Msg("quoting account not found")
		}
	}

	if xarbConfig.HedgingAccount != "" {
		hedgingAccount := x.GetCatalog().GetAccountByName(xarbConfig.HedgingAccount)
		if hedgingAccount != nil {
			x.hedgingAccount = *hedgingAccount
			log().Info().Msgf("Hedging account: %s(%d)", hedgingAccount.Name, hedgingAccount.ID)
		} else {
			log().Warn().Str("account", xarbConfig.HedgingAccount).Msg("hedging account not found")
		}
	}
}

// OnStart is called when the strategy starts.
// Note: Data subscriptions are now handled by the config - no manual Subscribe/Connect needed.
func (x *XArb) OnStart() {
	log().Info().Msgf("XArb strategy started for symbols: %s, %s",
		x.quotingSymbol.UniversalTicker, x.hedgingSymbol.UniversalTicker)
}

// OnStop is called when the strategy stops.
func (x *XArb) OnStop() {
	log().Info().Msg("XArb strategy stopped")
}

// Handle overrides StrategyBase.Handle to dispatch events to XArb's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (x *XArb) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		x.OnDepthUpdate(update)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := event.NewFillFromBytes(buf)
		x.OnFill(fill)
	case event.TopicEventPartialFill:
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

}

// OnRespDepthSnapshot processes the response to a depth snapshot request.
func (x *XArb) OnRespDepthSnapshot(snapshot event.RespDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log().Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asks", len(snapshot.Asks)).
		Int("bids", len(snapshot.Bids)).
		Msg("RespDepthSnapshot received")
}

// OnFill processes fill events.
func (x *XArb) OnFill(fill event.Fill) {}

func (x *XArb) OnPartialFill(partialFill event.OrderPartiallyFilled) {}

func (x *XArb) OnOrderCanceled(orderCanceled event.OrderCanceled) {}

func (x *XArb) OnOrderRejected(orderRejected event.OrderRejected) {}

func (x *XArb) OnOrderError(orderError event.OrderError) {}

func (x *XArb) OnOrderRiskInvalid(orderRiskInvalid event.OrderRiskInvalid) {}

func (x *XArb) OnOrderNew(orderNew event.OrderNew) {}

func (x *XArb) OnOrderAccepted(orderAccepted event.OrderAccepted) {}
