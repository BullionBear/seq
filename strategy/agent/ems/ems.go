package ems

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/strategy"
)

type EMS struct {
	strategy.StrategyCommon
}

func NewEMS() *EMS {
	return &EMS{}
}

func (ems *EMS) OnInit(config *strategy.StrategyConfig) error {
	return nil
}

func (ems *EMS) OnStart() error {
	return nil
}

func (ems *EMS) OnReady() error {
	return nil
}

func (ems *EMS) OnStop() error {
	return nil
}

func (ems *EMS) OnDispose() error {
	return nil
}

func (ems *EMS) OnDepthUpdate(depthUpdate event.DepthUpdate) error {
	return nil
}
