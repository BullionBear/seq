package node

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/internal/adapter"
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

	// Execution router for managing execution clients
	executionRouter *adapter.ExecutionRouter

	// Strategy
	strategyEngine *strategy.Engine

	// Cache for strategy access
	cache *Cache
}

// NewNode creates a new Node with the given catalog.
func NewNode(cat *catalog.Catalog) *Node {
	eventBus := evbus.NewEventBus()

	// Create execution router
	executionRouter := adapter.NewExecutionRouter()

	// Create engines
	dataEngine := data.NewEngine(cat, eventBus)
	riskEngine := risk.NewEngine()
	portfolioEngine := portfolio.NewEngine(eventBus)
	executionEngine := execution.NewEngine(executionRouter, eventBus)

	// Create cache with all engines
	cache := NewCache(dataEngine, executionEngine, portfolioEngine)

	return &Node{
		eventBus:        eventBus,
		catalog:         cat,
		dataEngine:      dataEngine,
		riskEngine:      riskEngine,
		portfolioEngine: portfolioEngine,
		executionEngine: executionEngine,
		executionRouter: executionRouter,
		cache:           cache,
	}
}

// Init initializes the node and all engines.
func (n *Node) Init(config *strategy.StrategyConfig, strategyActor actor.Actor) {
	// Initialize data engine (registers OrderBook actor)
	n.dataEngine.Init()

	// Register data engine as an actor for handling depth events
	actor.Register(n.eventBus, n.dataEngine)

	// Configure data engine with subscriptions from config
	if len(config.Data) > 0 {
		if err := n.dataEngine.SetDataConfig(config.Data); err != nil {
			log().Error().Err(err).Msg("Node: Failed to configure data subscriptions")
		}
	}

	// Configure execution engine with accounts from config
	if len(config.Execution.Accounts) > 0 {
		accountIDs := n.resolveAccountIDs(config.Execution.Accounts)
		n.executionEngine.SetAccounts(accountIDs)
	}

	// Create strategy engine and initialize it
	n.strategyEngine = strategy.NewEngine(strategyActor, n.catalog, n.cache)
	n.strategyEngine.Init(config, n.eventBus)

	log().Info().Msg("Node initialized")
}

// resolveAccountIDs resolves account names to account IDs using the catalog.
func (n *Node) resolveAccountIDs(accountNames []string) []int {
	accountIDs := make([]int, 0, len(accountNames))
	for _, name := range accountNames {
		account := n.catalog.GetAccountByName(name)
		if account != nil {
			accountIDs = append(accountIDs, account.ID)
			log().Debug().Str("name", name).Int("id", account.ID).Msg("Node: Resolved account")
		} else {
			log().Warn().Str("name", name).Msg("Node: Account not found in catalog")
		}
	}
	return accountIDs
}

// Start connects engines and starts the strategy.
func (n *Node) Start(ctx context.Context) {
	// Connect data engine (subscribes to configured streams and connects)
	n.dataEngine.Connect(ctx)
	log().Info().Msg("Node: DataEngine connected")

	// Connect execution engine
	if err := n.executionEngine.Connect(ctx); err != nil {
		log().Error().Err(err).Msg("Node: Failed to connect execution engine")
	} else {
		log().Info().Msg("Node: ExecutionEngine connected")
	}

	// Start strategy (now just calls OnStart for business logic)
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
	n.dataEngine.Disconnect()
	n.executionEngine.Disconnect()
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

// ExecutionRouter returns the execution router for registering execution clients.
func (n *Node) ExecutionRouter() *adapter.ExecutionRouter {
	return n.executionRouter
}

// ExecutionEngine returns the execution engine.
func (n *Node) ExecutionEngine() *execution.Engine {
	return n.executionEngine
}

// PortfolioEngine returns the portfolio engine.
func (n *Node) PortfolioEngine() *portfolio.Engine {
	return n.portfolioEngine
}
