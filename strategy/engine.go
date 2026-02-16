package strategy

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/msgbus"
)

// Engine manages strategy lifecycle.
// It focuses on strategy initialization and lifecycle management.
// The event loop is owned by the Node, not the Engine.
type Engine struct {
	strategyActor actor.Actor
	catalog       *catalog.Catalog
	cache         *cache.Cache
}

// NewEngine creates a new Engine with the given strategy actor, catalog, and cache.
// The strategy must implement the Actor interface (e.g., by embedding StrategyBase).
func NewEngine(strat actor.Actor, cat *catalog.Catalog, cache *cache.Cache) *Engine {
	return &Engine{
		strategyActor: strat,
		catalog:       cat,
		cache:         cache,
	}
}

// Init initializes the engine and the strategy actor.
// strategyConfig is the strategy-specific configuration map from the YAML entry.
func (e *Engine) Init(strategyConfig map[string]any, msgBus *msgbus.MsgBus) {
	// Register the strategy actor with the MsgBus
	actor.Register(msgBus, e.strategyActor)

	// Call OnInit on the strategy actor with strategy-specific config
	e.strategyActor.OnInit(strategyConfig)
}

// Start starts the strategy actor.
func (e *Engine) Start() {
	e.strategyActor.OnStart()
}

// Stop performs graceful shutdown of the strategy actor.
func (e *Engine) Stop() {
	e.strategyActor.OnStop()
}
