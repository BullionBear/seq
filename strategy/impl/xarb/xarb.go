package xarb

import (
	"context"
	"time"

	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
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
	x.SubscribeDepthDelta(x.quotingSymbol.ID)
	log.Info().Msgf("Subscribed to depth delta for quoting symbol: %s(%d)", x.quotingSymbol.UniversalTicker, x.quotingSymbol.ID)
	x.SubscribeDepthDelta(x.hedgingSymbol.ID)
	log.Info().Msgf("Subscribed to depth delta for hedging symbol: %s(%d)", x.hedgingSymbol.UniversalTicker, x.hedgingSymbol.ID)
	x.Connect(context.Background())
}

// OnStop disconnects from market data.
func (x *XArb) OnStop() {
	x.Disconnect()
}

// OnDepthUpdate processes depth updates.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	log.Info().Msgf("Depth update: %d %d %d %d", update.SymbolID, update.DepthID, len(update.Bids), len(update.Asks))
	log.Info().Msgf("Depth update timestamp: %s", time.Unix(int64(update.Timestamp/1_000_000_000), int64(update.Timestamp%1_000_000_000)).Format(time.RFC3339Nano))
}
