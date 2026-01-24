package xarb

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/strategy"
)

var _ strategy.Strategy = &XArb{}

type XArb struct {
	strategy.StrategyCommon
	quotingSymbolID int
	hedgingSymbolID int
}

func NewXArb() *XArb {
	return &XArb{
		quotingSymbolID: 0,
		hedgingSymbolID: 0,
	}
}

func (x *XArb) OnInit(config *strategy.StrategyConfig) error {
	xarbConfig := config.Strategy["strategy"].(XArbConfig)
	x.quotingSymbolID = xarbConfig.QuotingSymbolID
	x.hedgingSymbolID = xarbConfig.HedgingSymbolID
	return nil
}

func (x *XArb) OnStart() error {
	x.SubscribeDepthDelta(x.quotingSymbolID)
	x.SubscribeDepthDelta(x.hedgingSymbolID)
	return nil
}

func (x *XArb) OnReady() error {
	return nil
}

func (x *XArb) OnStop() error {
	x.UnsubscribeDepthDelta(x.quotingSymbolID)
	x.UnsubscribeDepthDelta(x.hedgingSymbolID)
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
