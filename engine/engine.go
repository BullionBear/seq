package engine

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
)

var log = logger.Get()

type Engine struct {
	eventBus *evbus.EventBus
	catalog  *catalog.Catalog
	strategy strategy.Strategy
}

func NewEngine(strat strategy.Strategy, cat *catalog.Catalog) *Engine {
	return &Engine{
		eventBus: evbus.NewEventBus(),
		strategy: strat,
		catalog:  cat,
	}
}

func (e *Engine) Init(config *strategy.StrategyConfig) {
	// Inject StrategyCommon into the strategy before initialization
	common := strategy.NewStrategyCommon(e.catalog, e.eventBus)
	e.strategy.SetCommon(common)
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

func (e *Engine) dispatch(ev evbus.Event) {
	switch ev.Ref.DataType {
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
				hasWork := e.eventBus.Poll(e.dispatch)
				if !hasWork {
					runtime.Gosched()
				}
			}
		}
	}()

	<-ctx.Done()
	e.stop()
}
