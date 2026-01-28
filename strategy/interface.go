package strategy

import "github.com/BullionBear/seq/core/model/event"

type Strategy interface {
	// SetCommon injects the StrategyCommon into the strategy
	SetCommon(common *StrategyCommon)
	// lifecycle methods
	OnInit(config *StrategyConfig) error // called once when the strategy is initialized
	OnStart() error                      // called once when the strategy is started
	OnReady() error                      // called once when the strategy is ready to start
	OnStop() error                       // called once when the strategy is stopped
	OnDispose() error                    // called once when the strategy is disposed
	// event handlers
	OnDepthUpdate(depthUpdate event.DepthUpdate) error
	OnTick(tick event.Tick) error
	// order event handlers
	OnOrderUpdate(orderUpdate event.OrderUpdate) error
	OnFill(fill event.Fill) error
	// balance event handlers
	OnBalanceUpdate(balanceUpdate event.BalanceUpdate) error
}
