package engine

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
)

type Engine struct {
	log      zerolog.Logger
	eventBus evbus.EventBus
	strategy strategy.Strategy
}

func NewEngine(strategy strategy.Strategy) *Engine {
	return &Engine{
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

func (e *Engine) handle(ev evbus.Event) {
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		depthSnapshot := e.eventBus.ReadDepthSnapshot(ev.Ref.Index)
		e.strategy.OnDepthSnapshot(depthSnapshot)
	case event.DataTypeDepthUpdate:
		depthUpdate := e.eventBus.ReadDepthUpdate(ev.Ref.Index)
		e.strategy.OnDepthUpdate(depthUpdate)
	case event.DataTypeTick:
		tick := e.eventBus.ReadTick(ev.Ref.Index)
		e.strategy.OnTick(tick)
	case event.DataTypeOrderUpdate:
		orderUpdate := e.eventBus.ReadOrderUpdate(ev.Ref.Index)
		e.strategy.OnOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		fill := e.eventBus.ReadFill(ev.Ref.Index)
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
