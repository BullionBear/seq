package node

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/portfolio"
	"github.com/BullionBear/seq/risk"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Node orchestrates all engines and the event loop.
// It owns the EventBus and provides a Cache for strategies to access data.
type Node struct {
	eventBus *evbus.EventBus
	catalog  *catalog.Catalog

	// Engines
	dataEngine      *data.Engine
	riskEngine      *risk.Engine
	portfolioEngine *portfolio.Engine
	executionEngine *execution.Engine

	// Strategy
	strategyEngine *strategy.Engine

	// Cache for strategy access
	cache *Cache
}

// NewNode creates a new Node with the given catalog.
func NewNode(cat *catalog.Catalog) *Node {
	eventBus := evbus.NewEventBus()

	// Create engines
	dataEngine := data.NewEngine(cat, eventBus)
	riskEngine := risk.NewEngine()
	portfolioEngine := portfolio.NewEngine()
	executionEngine := execution.NewEngine()

	// Create cache
	cache := NewCache(dataEngine)

	return &Node{
		eventBus:        eventBus,
		catalog:         cat,
		dataEngine:      dataEngine,
		riskEngine:      riskEngine,
		portfolioEngine: portfolioEngine,
		executionEngine: executionEngine,
		cache:           cache,
	}
}

// Init initializes the node and all engines.
func (n *Node) Init(config *strategy.StrategyConfig, strategyActor actor.Actor) {
	// Initialize data engine (registers OrderBook actor)
	n.dataEngine.Init()

	// Register data engine as an actor for handling depth events
	actor.Register(n.eventBus, n.dataEngine)

	// Create strategy engine and initialize it
	n.strategyEngine = strategy.NewEngine(strategyActor, n.catalog, n.cache)
	n.strategyEngine.Init(config, n.eventBus)

	log().Info().Msg("Node initialized")
}

// Start starts all engines and the strategy.
func (n *Node) Start() {
	n.strategyEngine.Start()
	log().Info().Msg("Node started")
}

// Run starts the event loop and processes events until context is cancelled.
func (n *Node) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				hasWork := n.eventBus.Dispatch()
				if hasWork {
					// Update minSequence and release arena memory
					n.eventBus.Release()
					n.eventBus.ReleaseArenas()
				} else {
					runtime.Gosched()
				}
			}
		}
	}()

	<-ctx.Done()
	n.stop()
}

// stop performs graceful shutdown of all engines.
func (n *Node) stop() {
	n.strategyEngine.Stop()
	log().Info().Msg("Node stopped")
}

// EventBus returns the node's EventBus for external access.
func (n *Node) EventBus() *evbus.EventBus {
	return n.eventBus
}

// Cache returns the node's Cache for strategy access.
func (n *Node) Cache() *Cache {
	return n.cache
}

// DataEngine returns the data engine.
func (n *Node) DataEngine() *data.Engine {
	return n.dataEngine
}
