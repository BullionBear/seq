package xarb

import (
	"fmt"

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

func (x *XArb) OnInit(config *strategy.StrategyConfig) error {
	// Convert map[string]any to XArbConfig struct via YAML re-marshaling
	var xarbConfig XArbConfig
	yamlData, err := yaml.Marshal(config.Strategy)
	if err != nil {
		return fmt.Errorf("failed to marshal strategy config: %w", err)
	}
	if err := yaml.Unmarshal(yamlData, &xarbConfig); err != nil {
		return fmt.Errorf("failed to unmarshal strategy config: %w", err)
	}
	quotingSymbol, err := x.GetCatalog().GetSymbolByName(xarbConfig.QuotingSymbolName)
	if err != nil {
		return err
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByName(xarbConfig.HedgingSymbolName)
	if err != nil {
		return err
	}
	x.logger.Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.Name, quotingSymbol.ID)
	x.logger.Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.Name, hedgingSymbol.ID)
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
	return nil
}

func (x *XArb) OnStart() error {
	x.SubscribeDepthDelta(x.quotingSymbol.ID)
	x.SubscribeDepthDelta(x.hedgingSymbol.ID)
	return nil
}

func (x *XArb) OnReady() error {
	return nil
}

func (x *XArb) OnStop() error {
	x.UnsubscribeDepthDelta(x.quotingSymbol.ID)
	x.UnsubscribeDepthDelta(x.hedgingSymbol.ID)
	return nil
}

func (x *XArb) OnDispose() error {
	return nil
}

func (x *XArb) OnDepthUpdate(depthUpdate event.DepthUpdate) error {
	return nil
}

func (x *XArb) OnTick(tick event.Tick) error {
	return nil
}

func (x *XArb) OnOrderUpdate(orderUpdate event.OrderUpdate) error {
	return nil
}

func (x *XArb) OnFill(fill event.Fill) error {
	return nil
}
