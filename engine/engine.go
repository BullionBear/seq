package engine

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
)

type Engine struct {
	log      zerolog.Logger
	eventBus evbus.EventBus
	strategy strategy.Strategy
}

func NewEngine(strategy strategy.Strategy) Engine {
	return Engine{
		log:      logger.Get(),
		eventBus: evbus.NewEventBus(),
		strategy: strategy,
	}
}

func (e *Engine) Init(config *strategy.StrategyConfig) {
	e.strategy.OnInit(config)
}

func (e *Engine) Start() {
	e.strategy.OnStart()
	e.strategy.OnReady()
}

func (e *Engine) stop() {
	e.strategy.OnStop()
	e.strategy.OnDispose()
}

func (e *Engine) handle(event evbus.Event) {
	switch event.Ref.DataType {
	case model.DataTypeDepthSnapshot:
		depthSnapshot := e.eventBus.ReadDepthSnapshot(event.Ref.Index)
		e.strategy.OnDepthSnapshot(depthSnapshot)
	case model.DataTypeDepthUpdate:
		depthUpdate := e.eventBus.ReadDepthUpdate(event.Ref.Index)
		e.strategy.OnDepthUpdate(depthUpdate)
	case model.DataTypeTick:
		tick := e.eventBus.ReadTick(event.Ref.Index)
		e.strategy.OnTick(tick)
	case model.DataTypeOrderUpdate:
		orderUpdate := e.eventBus.ReadOrderUpdate(event.Ref.Index)
		e.strategy.OnOrderUpdate(orderUpdate)
	case model.DataTypeFill:
		fill := e.eventBus.ReadFill(event.Ref.Index)
		e.strategy.OnFill(fill)
	}
}

func (e *Engine) Run(ctx context.Context, config *strategy.StrategyConfig) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				hasWork := e.eventBus.Poll(e.handle)
				if !hasWork {
					runtime.Gosched()
				}
			}
		}
	}()

	<-ctx.Done()
	e.stop()
}
