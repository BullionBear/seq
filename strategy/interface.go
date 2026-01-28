package strategy

import (
	"github.com/BullionBear/seq/core/model/event"
)

type Strategy interface {
	// SetCommon injects the StrategyCommon into the strategy
	SetCommon(common *StrategyCommon)
	// lifecycle methods
	OnInit(config *StrategyConfig) // called once when the strategy is initialized
	OnStart()                      // called once when the strategy is started
	OnReady()                      // called once when the strategy is ready to start
	OnStop()                       // called once when the strategy is stopped
	OnDispose()                    // called once when the strategy is disposed
	// event handlers
	OnDepthUpdate(depthUpdate event.DepthUpdate)
	OnTick(tick event.Tick)
	// order event handlers
	OnOrderUpdate(orderUpdate event.OrderUpdate)
	OnFill(fill event.Fill)
	// balance event handlers
	OnBalanceUpdate(balanceUpdate event.BalanceUpdate)
}
