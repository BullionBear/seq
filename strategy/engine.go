package strategy

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/internal/evbus"
)

// Engine manages strategy lifecycle.
// It focuses on strategy initialization and lifecycle management.
// The event loop is owned by the Node, not the Engine.
type Engine struct {
	strategyActor actor.Actor
	catalog       *catalog.Catalog
	cache         CacheService
	config        *StrategyConfig
}

// NewEngine creates a new Engine with the given strategy actor, catalog, and cache.
// The strategy must implement the Actor interface (e.g., by embedding StrategyBase).
func NewEngine(strat actor.Actor, cat *catalog.Catalog, cache CacheService) *Engine {
	return &Engine{
		strategyActor: strat,
		catalog:       cat,
		cache:         cache,
	}
}

// Init initializes the engine and the strategy actor.
// It creates StrategyCommon and injects it into the strategy.
func (e *Engine) Init(config *StrategyConfig, eventBus *evbus.EventBus) {
	e.config = config

	// Create StrategyCommon which wraps the cache
	common := NewStrategyCommon(e.cache, e.catalog)

	// Inject StrategyCommon into the strategy actor if it supports SetCommon
	if s, ok := e.strategyActor.(interface {
		SetCommon(*StrategyCommon)
	}); ok {
		s.SetCommon(common)
	}

	// Inject config into the strategy actor if it supports SetConfig
	if s, ok := e.strategyActor.(interface {
		SetConfig(*StrategyConfig)
	}); ok {
		s.SetConfig(config)
	}

	// Register the strategy actor with the EventBus
	actor.Register(eventBus, e.strategyActor)

	// Call OnInit on the strategy actor
	e.strategyActor.OnInit()
}

// Start starts the strategy actor.
func (e *Engine) Start() {
	e.strategyActor.OnStart()
}

// Stop performs graceful shutdown of the strategy actor.
func (e *Engine) Stop() {
	e.strategyActor.OnStop()
}
