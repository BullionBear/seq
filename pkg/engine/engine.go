package engine

import (
	"context"

	"github.com/BullionBear/seq/pkg/strategy"
)

type Engine struct {
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Run(ctx context.Context, strategy strategy.Strategy, config *strategy.StrategyConfig) {
	strategy.OnInit(config)
	strategy.OnStart()
	strategy.OnReady()

	<-ctx.Done()
	strategy.OnStop()
	strategy.OnDispose()
}
