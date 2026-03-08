package node

import (
	"context"
	"fmt"
	"runtime"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/adapter/binance"
	"github.com/BullionBear/seq/adapter/bybit"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/portfolio"
	"github.com/BullionBear/seq/risk"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Node orchestrates all engines and the event loop.
type Node struct {
	msgBus  *msgbus.MsgBus
	catalog *catalog.Catalog

	// Engines
	dataEngine      *data.Engine
	riskEngine      *risk.Engine
	portfolioEngine *portfolio.Engine
	executionEngine *execution.Engine
	strategyEngine  *strategy.Engine

	// Execution router for managing execution clients
	executionRouter *adapter.ExecutionRouter

	// Cache for strategy access
	cache *cache.Cache
}

// NewNode creates a new Node with the given catalog.
func NewNode(cat *catalog.Catalog) *Node {
	bus := msgbus.NewMsgBus()
	executionRouter := adapter.NewExecutionRouter()
	c := cache.NewCache()

	return &Node{
		msgBus:          bus,
		catalog:         cat,
		dataEngine:      data.NewEngine(cat, bus, c),
		riskEngine:      risk.NewEngine(cat, bus, c),
		portfolioEngine: portfolio.NewEngine(bus),
		executionEngine: execution.NewEngine(executionRouter, bus, c),
		strategyEngine:  strategy.NewEngine(cat, bus, c),
		executionRouter: executionRouter,
		cache:           c,
	}
}

// Init initializes the node and all engines from config.
// execRouter and dataRouter are the top-level adapter configs parsed from YAML.
func (n *Node) Init(config Config, execRouter []adapter.ExecRouterEntry, dataRouter []adapter.DataRouterEntry) {
	notifier := msgbus.NewStateNotifier(n.msgBus)

	// Configure portfolio engine with execution router and notifier
	n.portfolioEngine.SetExecutionRouter(n.executionRouter)
	n.portfolioEngine.SetNotifier(notifier)

	// Set up execution clients from top-level execrouter config
	accountIDs := n.setupExecutionClients(execRouter)
	n.portfolioEngine.SetAccounts(accountIDs)

	// Initialize all engines with their configs
	n.dataEngine.Init(config.Engine.Data, dataRouter)
	n.executionEngine.Init(config.Engine.Execution)
	n.portfolioEngine.Init(config.Engine.Portfolio)
	n.riskEngine.Init()
	n.strategyEngine.Init(config.Engine.Strategy)

	log().Info().Msg("Node initialized")
}

// setupExecutionClients creates and registers execution clients from
// the top-level execrouter config entries.
// Returns the list of resolved account IDs for portfolio tracking.
func (n *Node) setupExecutionClients(entries []adapter.ExecRouterEntry) []int {
	accountIDs := make([]int, 0, len(entries))

	for _, entry := range entries {
		var account *cpanel.Account
		if entry.Account != "" {
			account = n.catalog.GetAccountByName(entry.Account)
		} else if entry.ID > 0 {
			var err error
			account, err = n.catalog.GetAccount(entry.ID)
			if err != nil {
				log().Error().Err(err).Int("id", entry.ID).Msg("Node: Failed to get account by ID")
				continue
			}
		}

		if account == nil {
			log().Warn().Str("account", entry.Account).Int("id", entry.ID).Msg("Node: Account not found")
			continue
		}

		accountIDs = append(accountIDs, account.ID)

		client, err := n.createExecutionClient(account, entry.API)
		if err != nil {
			log().Error().Err(err).Str("account", account.Name).Msg("Node: Failed to create execution client")
			continue
		}

		n.executionRouter.RegisterClient(account.ID, client)
		log().Info().
			Str("account", account.Name).
			Int("id", account.ID).
			Str("exchange", account.Exchange).
			Msg("Node: Registered execution client")
	}

	return accountIDs
}

// createExecutionClient creates an execution client for the given account.
func (n *Node) createExecutionClient(account *cpanel.Account, apiKeyName string) (adapter.ExecutionClient, error) {
	switch account.Exchange {
	case "BINANCE", "Binance":
		return binance.NewBinanceSpotExecutionClient(n.catalog, n.msgBus, account.ID, apiKeyName)
	case "BYBIT", "Bybit":
		return bybit.NewBybitExecutionClient(n.catalog, n.msgBus, account.ID, apiKeyName)
	default:
		return nil, fmt.Errorf("unsupported exchange: %s", account.Exchange)
	}
}

// Start connects engines and starts all actors.
func (n *Node) Start(ctx context.Context) {
	n.dataEngine.Start()
	log().Info().Msg("Node: DataEngine started")

	if err := n.executionEngine.Connect(ctx); err != nil {
		log().Error().Err(err).Msg("Node: Failed to connect execution engine")
	} else {
		log().Info().Msg("Node: ExecutionEngine connected")
	}
	n.executionEngine.Start()

	n.portfolioEngine.Start()
	log().Info().Msg("Node: PortfolioEngine started")

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
				hasWork := n.msgBus.Dispatch()
				if hasWork {
					n.msgBus.Release()
					n.msgBus.ReleaseArenas()
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

// MsgBus returns the node's MsgBus.
func (n *Node) MsgBus() *msgbus.MsgBus {
	return n.msgBus
}

// Cache returns the node's Cache.
func (n *Node) Cache() *cache.Cache {
	return n.cache
}

// DataEngine returns the data engine.
func (n *Node) DataEngine() *data.Engine {
	return n.dataEngine
}

// ExecutionRouter returns the execution router.
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
