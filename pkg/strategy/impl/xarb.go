package impl

import (
	"github.com/BullionBear/seq/pkg/model"
	"github.com/BullionBear/seq/pkg/strategy"
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

func (x *XArb) OnDepth(depth *model.DepthUpdate) error {
	return nil
}

func (x *XArb) OnTick(tick *model.Tick) error {
	return nil
}

func (x *XArb) OnOrderUpdate(orderUpdate *model.OrderUpdate) error {
	return nil
}

func (x *XArb) OnOrderFill(orderFill *model.OrderFill) error {
	return nil
}
