package impl

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/strategy"
)

var _ strategy.Strategy = &XArb{}

type XArb struct {
	strategy.StrategyCommon
}

func NewXArb() *XArb {
	return &XArb{}
}

func (x *XArb) OnInit(config *strategy.StrategyConfig) error {
	return nil
}

func (x *XArb) OnStart() error {
	return nil
}

func (x *XArb) OnReady() error {
	return nil
}

func (x *XArb) OnStop() error {
	return nil
}

func (x *XArb) OnDispose() error {
	return nil
}

func (x *XArb) OnDepthSnapshot(depthSnapshot event.DepthSnapshot) error {
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
