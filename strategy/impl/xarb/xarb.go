package xarb

import (
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

var _ strategy.Strategy = &XArb{}

type XArb struct {
	*strategy.StrategyCommon
	logger        zerolog.Logger
	quotingSymbol cpanel.Symbol
	hedgingSymbol cpanel.Symbol
}

func NewXArb() *XArb {
	return &XArb{
		logger:        logger.Get(),
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
		x.logger.Error().Err(err).Msg("failed to marshal strategy config")
	}
	if err := yaml.Unmarshal(yamlData, &xarbConfig); err != nil {
		x.logger.Error().Err(err).Msg("failed to unmarshal strategy config")
	}
	quotingSymbol, err := x.GetCatalog().GetSymbolByName(xarbConfig.QuotingSymbolName)
	if err != nil {
		x.logger.Error().Err(err).Msg("failed to get quoting symbol")
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByName(xarbConfig.HedgingSymbolName)
	if err != nil {
		x.logger.Error().Err(err).Msg("failed to get hedging symbol")
	}
	x.logger.Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.Name, quotingSymbol.ID)
	x.logger.Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.Name, hedgingSymbol.ID)
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
}

func (x *XArb) OnStart() {
	x.SubscribeDepthDelta(x.quotingSymbol.ID)
	x.SubscribeDepthDelta(x.hedgingSymbol.ID)
}

func (x *XArb) OnReady() {
}

func (x *XArb) OnStop() {
	x.UnsubscribeDepthDelta(x.quotingSymbol.ID)
	x.UnsubscribeDepthDelta(x.hedgingSymbol.ID)
}

func (x *XArb) OnDispose() {
}

func (x *XArb) OnDepthUpdate(depthUpdate event.DepthUpdate) {
}

func (x *XArb) OnTick(tick event.Tick) {
}

func (x *XArb) OnOrderUpdate(orderUpdate event.OrderUpdate) {
}

func (x *XArb) OnFill(fill event.Fill) {
}
