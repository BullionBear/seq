package node

import (
	"context"
	"runtime"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/adapter/binance"
	"github.com/BullionBear/seq/adapter/bybit"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
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
// It owns the EventBus and provides a Cache for strategies to access data.
type Node struct {
	msgBus  *msgbus.MsgBus
	catalog *catalog.Catalog

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
	cache *cache.Cache
}

// NewNode creates a new Node with the given catalog.
func NewNode(cat *catalog.Catalog) *Node {
	bus := msgbus.NewMsgBus()

	// Create execution router
	executionRouter := adapter.NewExecutionRouter()

	// Create cache (self-contained, no engine dependencies)
	c := cache.NewCache()

	// Create engines
	dataEngine := data.NewEngine(cat, bus, c)
	riskEngine := risk.NewEngine()
	portfolioEngine := portfolio.NewEngine(bus)
	executionEngine := execution.NewEngine(executionRouter, bus)

	return &Node{
		msgBus:          bus,
		catalog:         cat,
		dataEngine:      dataEngine,
		riskEngine:      riskEngine,
		portfolioEngine: portfolioEngine,
		executionEngine: executionEngine,
		executionRouter: executionRouter,
		cache:           c,
	}
}

// Init initializes the node and all engines.
func (n *Node) Init(config *strategy.StrategyConfig, strategyActor actor.Actor) {
	// Create state notifier for engines to broadcast state events
	notifier := msgbus.NewStateNotifier(n.msgBus)

	// Initialize data engine (registers OrderBook actor)
	n.dataEngine.Init()

	// Configure portfolio engine with execution router and notifier
	n.portfolioEngine.SetExecutionRouter(n.executionRouter)
	n.portfolioEngine.SetNotifier(notifier)

	// Configure data engine with subscriptions from config
	if len(config.Data) > 0 {
		if err := n.dataEngine.SetDataConfig(config.Data); err != nil {
			log().Error().Err(err).Msg("Node: Failed to configure data subscriptions")
		}
	}

	// Configure execution and portfolio with accounts from config
	if len(config.Execution) > 0 {
		n.setupExecutionClients(config.Execution)
	}

	// Initialize portfolio engine (subscribes to balance updates)
	n.portfolioEngine.Init()

	// Create strategy engine and initialize it
	n.strategyEngine = strategy.NewEngine(strategyActor, n.catalog, n.cache)
	n.strategyEngine.Init(config, n.msgBus)

	log().Info().Msg("Node initialized")
}

// setupExecutionClients creates and registers execution clients for configured accounts.
func (n *Node) setupExecutionClients(execConfig strategy.ConfigExecution) {
	accountIDs := make([]int, 0, len(execConfig))

	for _, cfg := range execConfig {
		// Resolve account from catalog
		var account *cpanel.Account
		if cfg.Account != "" {
			account = n.catalog.GetAccountByName(cfg.Account)
		} else if cfg.ID > 0 {
			var err error
			account, err = n.catalog.GetAccount(cfg.ID)
			if err != nil {
				log().Error().Err(err).Int("id", cfg.ID).Msg("Node: Failed to get account by ID")
				continue
			}
		}

		if account == nil {
			log().Warn().Str("account", cfg.Account).Int("id", cfg.ID).Msg("Node: Account not found")
			continue
		}

		accountIDs = append(accountIDs, account.ID)

		// Create execution client based on exchange
		client, err := n.createExecutionClient(account)
		if err != nil {
			log().Error().Err(err).Str("account", account.Name).Msg("Node: Failed to create execution client")
			continue
		}

		// Register client with router
		n.executionRouter.RegisterClient(account.ID, client)
		log().Info().
			Str("account", account.Name).
			Int("id", account.ID).
			Str("exchange", account.Exchange).
			Msg("Node: Registered execution client")
	}

	// Set configured accounts on execution engine and portfolio engine
	n.executionEngine.SetAccounts(accountIDs)
	n.portfolioEngine.SetAccounts(accountIDs)
}

// createExecutionClient creates an execution client for the given account based on exchange.
func (n *Node) createExecutionClient(account *cpanel.Account) (adapter.ExecutionClient, error) {
	switch account.Exchange {
	case "BINANCE":
		client, err := binance.NewBinanceSpotExecutionClient(n.catalog, n.msgBus, account.ID)
		if err != nil {
			return nil, err
		}
		return client, nil
	case "BYBIT":
		client, err := bybit.NewBybitExecutionClient(n.catalog, n.msgBus, account.ID)
		if err != nil {
			return nil, err
		}
		return client, nil
	default:
		log().Warn().Str("exchange", account.Exchange).Msg("Node: Unsupported exchange for execution")
		return nil, nil
	}
}

// getExchangeID returns the common.Exchange ID for an exchange name.
func getExchangeID(exchange string) int {
	switch exchange {
	case "BINANCE":
		return int(common.ExchangeBinance)
	case "BYBIT":
		return int(common.ExchangeBybit)
	default:
		return -1
	}
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

	// Start portfolio engine (requests balance snapshots, notifies ready when complete)
	n.portfolioEngine.Start()
	log().Info().Msg("Node: PortfolioEngine started")

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
				hasWork := n.msgBus.Dispatch()
				if hasWork {
					// Update minSequence and release arena memory
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

// MsgBus returns the node's MsgBus for external access.
func (n *Node) MsgBus() *msgbus.MsgBus {
	return n.msgBus
}

// Cache returns the node's Cache for strategy access.
func (n *Node) Cache() *cache.Cache {
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
