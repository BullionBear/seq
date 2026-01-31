package xarb

import (
	"context"
	"time"

	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/BullionBear/seq/strategy/actor"
	"gopkg.in/yaml.v3"
)

var log = logger.Get()

// Ensure XArb implements the Actor interface
var _ actor.Actor = (*XArb)(nil)

// XArb is a cross-exchange arbitrage strategy.
type XArb struct {
	*strategy.StrategyBase // Embed StrategyBase for Actor + StrategyCommon
	quotingSymbol          cpanel.Symbol
	hedgingSymbol          cpanel.Symbol
}

// NewXArb creates a new XArb strategy.
func NewXArb() *XArb {
	return &XArb{
		StrategyBase:  strategy.NewStrategyBase("xarb"),
		quotingSymbol: cpanel.Symbol{},
		hedgingSymbol: cpanel.Symbol{},
	}
}

// OnInit initializes the strategy with configuration.
func (x *XArb) OnInit() {
	// Get strategy-specific config from StrategyBase
	strategyConfig := x.StrategyConfig()
	if strategyConfig == nil {
		log.Error().Msg("strategy config is nil")
		return
	}

	// Convert map[string]any to XArbConfig struct via YAML re-marshaling
	var xarbConfig XArbConfig
	yamlData, err := yaml.Marshal(strategyConfig)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal strategy config")
		return
	}
	if err := yaml.Unmarshal(yamlData, &xarbConfig); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal strategy config")
		return
	}

	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.QuotingSymbolUniversalTicker)
	if err != nil {
		log.Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.HedgingSymbolUniversalTicker)
	if err != nil {
		log.Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	log.Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.UniversalTicker, quotingSymbol.ID)
	log.Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.UniversalTicker, hedgingSymbol.ID)
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
}

// OnStart subscribes to market data and connects.
func (x *XArb) OnStart() {
	x.SubscribeDepthUpdate(x.quotingSymbol.ID)
	log.Info().Msgf("Subscribed to depth update for quoting symbol: %s(%d)", x.quotingSymbol.UniversalTicker, x.quotingSymbol.ID)
	x.SubscribeDepthUpdate(x.hedgingSymbol.ID)
	log.Info().Msgf("Subscribed to depth update for hedging symbol: %s(%d)", x.hedgingSymbol.UniversalTicker, x.hedgingSymbol.ID)
	x.Connect(context.Background())

	// Request depth snapshots after connection is established
	// This initializes the OrderBook state so updates can be applied
	/*
		go func() {
			// Wait a bit for WebSocket to be fully connected
			time.Sleep(500 * time.Millisecond)

			if err := x.ReqDepthSnapshot(x.quotingSymbol.ID); err != nil {
				log.Error().Err(err).Msgf("Failed to request depth snapshot for quoting symbol: %d", x.quotingSymbol.ID)
			} else {
				log.Info().Msgf("Requested depth snapshot for quoting symbol: %s(%d)", x.quotingSymbol.UniversalTicker, x.quotingSymbol.ID)
			}

			if err := x.ReqDepthSnapshot(x.hedgingSymbol.ID); err != nil {
				log.Error().Err(err).Msgf("Failed to request depth snapshot for hedging symbol: %d", x.hedgingSymbol.ID)
			} else {
				log.Info().Msgf("Requested depth snapshot for hedging symbol: %s(%d)", x.hedgingSymbol.UniversalTicker, x.hedgingSymbol.ID)
			}
		}()
	*/
}

// OnStop disconnects from market data.
func (x *XArb) OnStop() {
	x.Disconnect()
}

// Handle overrides StrategyBase.Handle to dispatch events to XArb's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (x *XArb) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		x.OnDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		x.OnDepthUpdate(update)
	case event.DataTypeTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := evbus.DeserializeTick(buf)
		x.OnTick(tick)
	case event.DataTypeOrderUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderUpdate := evbus.DeserializeOrderUpdate(buf)
		x.OnOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		x.OnFill(fill)
	}
}

// OnDepthSnapshot processes depth snapshots.
func (x *XArb) OnDepthSnapshot(snapshot event.DepthSnapshot) {
	log.Info().Msgf("Depth snapshot: %d %d %d %d", snapshot.SymbolID, snapshot.DepthID, len(snapshot.Bids), len(snapshot.Asks))
}

// OnDepthUpdate processes depth updates.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	log.Info().Msgf("Depth update: %d %d %d %d", update.SymbolID, update.DepthID, len(update.Bids), len(update.Asks))
	log.Info().Msgf("Depth update timestamp: %s", time.Unix(int64(update.Timestamp/1_000_000_000), int64(update.Timestamp%1_000_000_000)).Format(time.RFC3339Nano))
}

// OnTick processes tick events.
func (x *XArb) OnTick(tick event.Tick) {}

// OnOrderUpdate processes order updates.
func (x *XArb) OnOrderUpdate(update event.OrderUpdate) {}

// OnFill processes fill events.
func (x *XArb) OnFill(fill event.Fill) {}
