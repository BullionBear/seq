package node

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/adapter/binance"
	"github.com/BullionBear/seq/adapter/binancefutures"
	"github.com/BullionBear/seq/adapter/bybit"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/clock"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/core/tradingmode"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/ledger"
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
	ledgerEngine    *ledger.Engine
	executionEngine *execution.Engine
	strategyEngine  *strategy.Engine

	// Execution router for managing execution clients
	executionRouter *adapter.ExecutionRouter

	// Cache for strategy access
	cache *cache.Cache

	// Dispatch-loop tuning (P2-4), captured from config in Init.
	dispatchCfg DispatchConfig
}

// NewNode creates a new Node with the given catalog.
func NewNode(cat *catalog.Catalog) *Node {
	bus := msgbus.NewMsgBus()
	bus.SetTicker(clock.NewClock(bus))
	executionRouter := adapter.NewExecutionRouter()
	c := cache.NewCache()

	return &Node{
		msgBus:          bus,
		catalog:         cat,
		dataEngine:      data.NewEngine(cat, bus, c),
		riskEngine:      risk.NewEngine(cat, bus, c),
		ledgerEngine:    ledger.NewEngine(cat, bus, c),
		executionEngine: execution.NewEngine(executionRouter, bus, c),
		strategyEngine:  strategy.NewEngine(cat, bus, c),
		executionRouter: executionRouter,
		cache:           c,
	}
}

// Init initializes the node and all engines from config.
// execRouter and dataRouter are the top-level adapter configs parsed from YAML.
// tradingMode gates venue order mutations on the execution router (default paper).
func (n *Node) Init(config Config, execRouter []adapter.ExecRouterEntry, dataRouter []adapter.DataRouterEntry, tradingMode tradingmode.Mode) {
	if tradingMode == "" {
		tradingMode = tradingmode.ModePaper
	}
	n.executionRouter.SetTradingMode(tradingMode)
	log().Info().
		Str("trading_mode", tradingMode.String()).
		Bool("live_orders_enabled", tradingMode.IsLive()).
		Msg("Trading mode active: venue order submit/cancel require live mode")

	n.dispatchCfg = config.Dispatch
	if n.dispatchCfg.IdleStrategy == "" {
		n.dispatchCfg.IdleStrategy = IdleStrategyGosched
	}
	if n.dispatchCfg.SpinBudget <= 0 {
		n.dispatchCfg.SpinBudget = DefaultSpinBudget
	}
	log().Info().
		Str("idle_strategy", n.dispatchCfg.IdleStrategy).
		Int("spin_budget", n.dispatchCfg.SpinBudget).
		Msg("Dispatch loop configured (OS-thread pinned)")

	notifier := msgbus.NewStateNotifier(n.msgBus)

	// Configure ledger engine with execution router and notifier
	n.ledgerEngine.SetExecutionRouter(n.executionRouter)
	n.ledgerEngine.SetNotifier(notifier)

	// Set up execution clients from top-level execrouter config
	accountIDs, walletTypes := n.setupExecutionClients(execRouter)
	n.ledgerEngine.SetAccounts(accountIDs, walletTypes)

	// Initialize all engines with their configs
	n.dataEngine.Init(config.Engine.Data, dataRouter)
	n.executionEngine.Init(config.Engine.Execution)
	n.ledgerEngine.Init(config.Engine.Ledger)
	if err := n.riskEngine.Init(config.Engine.Risk); err != nil {
		log().Fatal().Err(err).Msg("Node: risk engine init failed")
	}
	n.strategyEngine.Init(config.Engine.Strategy)

	// Dispatch order is a correctness contract: cache writers (data /
	// execution / ledger) must precede readers (strategy). See
	// docs/CONSUMER_ORDER.md.
	if err := n.msgBus.AssertOrder(); err != nil {
		log().Fatal().Err(err).Msg("Node: consumer order assertion failed")
	}

	log().Info().Str("trading_mode", tradingMode.String()).Msg("Node initialized")
}

// setupExecutionClients creates and registers execution clients from
// the top-level execrouter config entries.
// Returns the list of resolved account IDs and a map of accountID -> WalletType.
func (n *Node) setupExecutionClients(entries []adapter.ExecRouterEntry) ([]int, map[int]common.WalletType) {
	accountIDs := make([]int, 0, len(entries))
	walletTypes := make(map[int]common.WalletType, len(entries))

	for _, entry := range entries {
		account := n.catalog.GetAccountByName(entry.Account)
		if account == nil {
			log().Warn().Str("account", entry.Account).Msg("Node: Account not found")
			continue
		}

		// Resolve wallet name to wallet ID and wallet type
		walletID := 0
		walletType := common.WalletTypeUnknown
		if entry.Wallet != "" {
			wallet, err := account.GetWallet(entry.Wallet)
			if err != nil {
				log().Warn().Err(err).Str("wallet", entry.Wallet).Str("account", account.Name).
					Msg("Node: Wallet not found, using walletID=0")
			} else {
				walletID = wallet.ID
				walletType = wallet.WalletType
			}
		}

		accountIDs = append(accountIDs, account.ID)
		walletTypes[account.ID] = walletType

		client, err := n.createExecutionClient(account, entry.API, walletID, walletType)
		if err != nil {
			log().Error().Err(err).Str("account", account.Name).Msg("Node: Failed to create execution client")
			continue
		}

		n.executionRouter.RegisterClient(account.ID, client)
		log().Info().
			Str("account", account.Name).
			Int("id", account.ID).
			Int("walletID", walletID).
			Str("walletType", walletType.String()).
			Str("exchange", account.Exchange).
			Msg("Node: Registered execution client")
	}

	return accountIDs, walletTypes
}

// createExecutionClient creates an execution client for the given account.
// For Binance, wallet type umargin selects the USD-M futures client; otherwise spot.
func (n *Node) createExecutionClient(account *catalog.Account, apiKeyName string, walletID int, walletType common.WalletType) (adapter.ExecutionClient, error) {
	switch account.Exchange {
	case "BINANCE", "Binance":
		if walletType == common.WalletTypeUMargin {
			return binancefutures.NewBinanceFuturesExecutionClient(n.catalog, n.msgBus, account.ID, apiKeyName, walletID)
		}
		return binance.NewBinanceSpotExecutionClient(n.catalog, n.msgBus, account.ID, apiKeyName, walletID)
	case "BYBIT", "Bybit":
		return bybit.NewBybitExecutionClient(n.catalog, n.msgBus, account.ID, apiKeyName, walletID)
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

	n.ledgerEngine.Start()
	log().Info().Msg("Node: LedgerEngine started")

	n.riskEngine.Start()
	log().Info().Msg("Node: RiskEngine started")

	n.strategyEngine.Start()
	log().Info().Msg("Node started")
}

// Run starts the event loop and blocks until context is cancelled,
// then performs graceful shutdown.
//
// The dispatch goroutine is pinned to its OS thread (P2-4) so the Go
// scheduler cannot migrate it across cores; combined with the taskset /
// isolcpus recipe in docs/DEPLOYMENT.md this gives the loop a stable,
// cache-warm core. The idle strategy is configurable: cooperative Gosched
// (default) or a bounded busy-spin for latency-critical deployments on a
// dedicated core.
func (n *Node) Run(ctx context.Context) {
	done := make(chan struct{})
	exited := make(chan struct{})

	// Low-frequency reporter for the hot-path overflow counters (P2-3).
	n.msgBus.StartObserver(ctx, msgbus.DefaultObserverInterval)

	spinStrategy := n.dispatchCfg.IdleStrategy == IdleStrategySpin
	spinBudget := n.dispatchCfg.SpinBudget
	if spinBudget <= 0 {
		spinBudget = DefaultSpinBudget
	}

	go func() {
		defer close(exited)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		idleSpins := 0
		for {
			select {
			case <-done:
				return
			default:
				if t := n.msgBus.GetTicker(); t != nil {
					t.Tick(uint64(time.Now().UnixNano()))
				}
				hasWork := n.msgBus.Dispatch()
				if hasWork {
					n.msgBus.Release()
					n.msgBus.ReleaseArenas()
					idleSpins = 0
				} else if spinStrategy {
					// Bounded busy-spin: keep the thread hot for the next
					// event, but yield periodically so this core is not
					// unconditionally monopolized if isolation is absent.
					idleSpins++
					if idleSpins >= spinBudget {
						idleSpins = 0
						runtime.Gosched()
					}
				} else {
					runtime.Gosched()
				}
			}
		}
	}()

	<-ctx.Done()
	close(done)
	<-exited
	n.stop()
}

// stop performs graceful shutdown in the correct order:
//  1. Strategy actors stop (send cancel-order commands to msgbus)
//  2. Data clients disconnect (stop incoming market data)
//  3. Drain msgbus (dispatch loop processes pending cancel commands
//     and receives exchange cancel confirmations)
//  4. Execution/ledger engines stop their actors
//  5. Execution clients disconnect
func (n *Node) stop() {
	log().Info().Msg("Node: shutting down...")

	n.strategyEngine.Stop()
	log().Info().Msg("Node: strategy engine stopped")

	n.dataEngine.Disconnect()
	log().Info().Msg("Node: data clients disconnected")

	n.drainMsgBus()

	n.riskEngine.Stop()
	n.executionEngine.Stop()
	n.ledgerEngine.Stop()
	log().Info().Msg("Node: engines stopped")

	n.executionEngine.Disconnect()
	log().Info().Msg("Node: execution clients disconnected")

	log().Info().Msg("Node stopped")
}

// drainMsgBus runs the dispatch loop until no work remains,
// with a timeout to avoid hanging indefinitely.
func (n *Node) drainMsgBus() {
	deadline := time.Now().Add(3 * time.Second)
	idleRounds := 0
	const maxIdleRounds = 100

	for time.Now().Before(deadline) {
		if t := n.msgBus.GetTicker(); t != nil {
			t.Tick(uint64(time.Now().UnixNano()))
		}
		hasWork := n.msgBus.Dispatch()
		if hasWork {
			n.msgBus.Release()
			n.msgBus.ReleaseArenas()
			idleRounds = 0
		} else {
			idleRounds++
			if idleRounds >= maxIdleRounds {
				break
			}
			runtime.Gosched()
		}
	}
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

// LedgerEngine returns the ledger engine.
func (n *Node) LedgerEngine() *ledger.Engine {
	return n.ledgerEngine
}
