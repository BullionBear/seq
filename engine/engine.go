package engine

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/BullionBear/seq/strategy/actor"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine orchestrates event dispatching to actors (including strategies).
// All event-driven components implement the unified Actor interface.
// The Engine focuses on the event loop while StrategyCommon manages the trading
// infrastructure (OrderBook, EMS, etc.) internally via the facade pattern.
type Engine struct {
	eventBus      *evbus.EventBus
	catalog       *catalog.Catalog
	strategyActor actor.Actor
	config        *strategy.StrategyConfig
}

// NewEngine creates a new Engine with the given strategy actor and catalog.
// The strategy must implement the Actor interface (e.g., by embedding StrategyBase).
func NewEngine(strat actor.Actor, cat *catalog.Catalog) *Engine {
	return &Engine{
		eventBus:      evbus.NewEventBus(),
		strategyActor: strat,
		catalog:       cat,
	}
}

// Init initializes the engine and all actors.
// StrategyCommon internally creates and registers infrastructure actors (OrderBook, EMS)
// with the EventBus during construction.
func (e *Engine) Init(config *strategy.StrategyConfig) {
	e.config = config

	// Create StrategyCommon which internally registers infrastructure actors
	common := strategy.NewStrategyCommon(e.catalog, e.eventBus)

	// Inject StrategyCommon into the strategy actor if it supports SetCommon
	if s, ok := e.strategyActor.(interface {
		SetCommon(*strategy.StrategyCommon)
	}); ok {
		s.SetCommon(common)
	}

	// Inject config into the strategy actor if it supports SetConfig
	if s, ok := e.strategyActor.(interface {
		SetConfig(*strategy.StrategyConfig)
	}); ok {
		s.SetConfig(config)
	}

	// Register the strategy actor with the EventBus
	actor.Register(e.eventBus, e.strategyActor)

	// Call OnInit on the strategy actor
	e.strategyActor.OnInit()
}

// Start starts all actors.
func (e *Engine) Start() {
	e.strategyActor.OnStart()
}

// stop performs graceful shutdown of all actors.
func (e *Engine) stop() {
	e.strategyActor.OnStop()
}

// Run starts the event loop and processes events until context is cancelled.
// Events are dispatched to all registered actors.
// After all consumers process each event, arena memory is released.
func (e *Engine) Run(ctx context.Context, config *strategy.StrategyConfig) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				hasWork := e.eventBus.Dispatch()
				if hasWork {
					// Update minSequence and release arena memory
					e.eventBus.Release()
					e.eventBus.ReleaseArenas()
				} else {
					runtime.Gosched()
				}
			}
		}
	}()

	<-ctx.Done()
	e.stop()
}

// EventBus returns the engine's EventBus for external access (e.g., publishing events).
func (e *Engine) EventBus() *evbus.EventBus {
	return e.eventBus
}
