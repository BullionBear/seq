package strategy

import "github.com/BullionBear/seq/core/model/event"

type Strategy interface {
	// lifecycle methods
	OnInit(config *StrategyConfig) error // called once when the strategy is initialized
	OnStart() error                      // called once when the strategy is started
	OnReady() error                      // called once when the strategy is ready to start
	OnStop() error                       // called once when the strategy is stopped
	OnDispose() error                    // called once when the strategy is disposed
	// event handlers
	// market data handlers
	OnDepthSnapshot(depthSnapshot event.DepthSnapshot) error
	OnDepthUpdate(depthUpdate event.DepthUpdate) error
	OnTick(tick event.Tick) error
	// execution handlers
	OnOrderUpdate(orderUpdate event.OrderUpdate) error
	OnFill(fill event.Fill) error
}
