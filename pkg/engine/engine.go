package engine

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/pkg/model"
	"github.com/BullionBear/seq/pkg/strategy"
)

type Engine struct {
	eventBus evbus.EventBus
}

func NewEngine() Engine {
	return Engine{
		eventBus: evbus.NewEventBus(),
	}
}

func (e *Engine) Run(ctx context.Context, strategy strategy.Strategy, config *strategy.StrategyConfig) {
	strategy.OnInit(config)
	strategy.OnStart()
	strategy.OnReady()
	go func() {
		for {
			hasWork := e.eventBus.Poll(func(event evbus.Event) {
				switch event.Ref.DataType {
				case model.DataTypeDepthSnapshot:
					depthSnapshot := e.eventBus.ReadDepthSnapshot(event.Ref.Index)
					strategy.OnDepthSnapshot(depthSnapshot)
				case model.DataTypeDepthUpdate:
					depthUpdate := e.eventBus.ReadDepthUpdate(event.Ref.Index)
					strategy.OnDepthUpdate(depthUpdate)
				case model.DataTypeTick:
					tick := e.eventBus.ReadTick(event.Ref.Index)
					strategy.OnTick(tick)
				case model.DataTypeOrderUpdate:
					orderUpdate := e.eventBus.ReadOrderUpdate(event.Ref.Index)
					strategy.OnOrderUpdate(orderUpdate)
				case model.DataTypeOrderFill:
					orderFill := e.eventBus.ReadOrderFill(event.Ref.Index)
					strategy.OnOrderFill(orderFill)
				}
			})

			if !hasWork {
				runtime.Gosched()
			}
		}
	}()

	<-ctx.Done()
	strategy.OnStop()
	strategy.OnDispose()
}
