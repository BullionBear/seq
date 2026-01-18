package strategy

import "github.com/BullionBear/seq/pkg/model"

type Strategy interface {
	// lifecycle methods
	OnInit(config *StrategyConfig) error // called once when the strategy is initialized
	OnStart() error                      // called once when the strategy is started
	OnReady() error                      // called once when the strategy is ready to start
	OnStop() error                       // called once when the strategy is stopped
	OnDispose() error                    // called once when the strategy is disposed
	// event handlers
	// market data handlers
	OnDepthSnapshot(depthSnapshot model.DepthSnapshot) error
	OnDepthUpdate(depthUpdate model.DepthUpdate) error
	OnTick(tick model.Tick) error
	// execution handlers
	OnOrderUpdate(orderUpdate model.OrderUpdate) error
	OnFill(fill model.Fill) error
}
