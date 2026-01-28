package xarb

import (
	"context"
	"time"

	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/strategy"
	"gopkg.in/yaml.v3"
)

var log = logger.Get()

var _ strategy.Strategy = &XArb{}

type XArb struct {
	*strategy.StrategyCommon
	quotingSymbol cpanel.Symbol
	hedgingSymbol cpanel.Symbol
}

func NewXArb() *XArb {
	return &XArb{
		quotingSymbol: cpanel.Symbol{},
		hedgingSymbol: cpanel.Symbol{},
	}
}

func (x *XArb) SetCommon(common *strategy.StrategyCommon) {
	x.StrategyCommon = common
}

func (x *XArb) OnInit(config *strategy.StrategyConfig) {
	// Convert map[string]any to XArbConfig struct via YAML re-marshaling
	var xarbConfig XArbConfig
	yamlData, err := yaml.Marshal(config.Strategy)
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

func (x *XArb) OnStart() {
	x.SubscribeDepthDelta(x.quotingSymbol.ID)
	log.Info().Msgf("Subscribed to depth delta for quoting symbol: %s(%d)", x.quotingSymbol.UniversalTicker, x.quotingSymbol.ID)
	x.SubscribeDepthDelta(x.hedgingSymbol.ID)
	log.Info().Msgf("Subscribed to depth delta for hedging symbol: %s(%d)", x.hedgingSymbol.UniversalTicker, x.hedgingSymbol.ID)
	x.Connect(context.Background())
}

func (x *XArb) OnReady() {
}

func (x *XArb) OnStop() {
	x.Disconnect()
}

func (x *XArb) OnDispose() {
}

func (x *XArb) OnDepthUpdate(depthUpdate event.DepthUpdate) {
	log.Info().Msgf("Depth update: %d %d %d %d", depthUpdate.SymbolID, depthUpdate.DepthID, len(depthUpdate.Bids), len(depthUpdate.Asks))
	log.Info().Msgf("Depth update timestamp: %s", time.Unix(int64(depthUpdate.Timestamp/1_000_000_000), int64(depthUpdate.Timestamp%1_000_000_000)).Format(time.RFC3339Nano))
}

func (x *XArb) OnTick(tick event.Tick) {
}

func (x *XArb) OnOrderUpdate(orderUpdate event.OrderUpdate) {
}

func (x *XArb) OnFill(fill event.Fill) {
}
