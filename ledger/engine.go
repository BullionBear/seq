package ledger

import (
	"sync"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages ledger actors and orchestrates their lifecycle.
// Balance state is owned by individual BalanceActors which write to cache.
type Engine struct {
	engine.EngineBase
	catalog *catalog.Catalog
	cache   *cache.Cache
	msgBus  *msgbus.MsgBus
	actors  []actor.Actor

	execRouter *adapter.ExecutionRouter

	accountIDs  []int
	walletTypes map[int]common.WalletType // accountID -> WalletType

	// Snapshot readiness: counts NotifyReady calls from balance actors
	pendingCount int
	readyCount   int
	ready        bool
	mu           sync.Mutex
}

// NewEngine creates a new ledger engine.
func NewEngine(cat *catalog.Catalog, msgBus *msgbus.MsgBus, c *cache.Cache) *Engine {
	return &Engine{
		EngineBase:  engine.NewEngineBase(common.EngineLedger),
		catalog:     cat,
		cache:       c,
		msgBus:      msgBus,
		accountIDs:  make([]int, 0),
		walletTypes: make(map[int]common.WalletType),
	}
}

// SetExecutionRouter sets the execution router for subscribing and requesting balance snapshots.
func (e *Engine) SetExecutionRouter(router *adapter.ExecutionRouter) {
	e.execRouter = router
}

// SetAccounts sets the account IDs and their wallet types to track.
func (e *Engine) SetAccounts(accountIDs []int, walletTypes map[int]common.WalletType) {
	e.accountIDs = accountIDs
	e.walletTypes = walletTypes

	log().Info().
		Ints("accountIDs", accountIDs).
		Msg("LedgerEngine: Configured accounts")
}

// GetConfiguredAccounts returns the list of configured account IDs.
func (e *Engine) GetConfiguredAccounts() []int {
	return e.accountIDs
}

// ============================================================================
// Engine Lifecycle
// ============================================================================

// Init constructs actors from config and registers with the EventBus.
func (e *Engine) Init(config Config) {
	balanceActorCount := 0

	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			log().Error().Err(err).Str("type", entry.Type).Msg("LedgerEngine: skipping unknown actor type")
			continue
		}

		a := factory(e)
		actor.ApplyName(a, entry.Name)

		// Inject cache and catalog into actors that support it
		type cacheSetter interface{ SetCache(*cache.Cache) }
		if cs, ok := a.(cacheSetter); ok {
			cs.SetCache(e.cache)
		}
		type catalogSetter interface{ SetCatalog(*catalog.Catalog) }
		if cs, ok := a.(catalogSetter); ok {
			cs.SetCatalog(e.catalog)
		}

		a.OnInit(entry.Config)
		actor.Register(e.msgBus, a)
		e.actors = append(e.actors, a)

		if entry.Type == "balance" {
			balanceActorCount++
		}

		log().Info().Str("type", entry.Type).Str("name", a.Name()).Msg("LedgerEngine: actor initialized")
	}

	e.mu.Lock()
	e.pendingCount = balanceActorCount
	e.mu.Unlock()

	if e.execRouter == nil {
		log().Warn().Msg("LedgerEngine: No execution router configured, skipping balance subscription")
	} else {
		for _, acctID := range e.accountIDs {
			if err := e.execRouter.SubscribeBalance(acctID); err != nil {
				log().Error().Err(err).Int("accountID", acctID).Msg("LedgerEngine: Failed to subscribe to balance updates")
			} else {
				log().Debug().Int("accountID", acctID).Msg("LedgerEngine: Subscribed to balance updates")
			}
		}
	}

	log().Info().Msg("LedgerEngine initialized")
}

// Start triggers actors and requests initial balance snapshots for all accounts.
func (e *Engine) Start() {
	for _, a := range e.actors {
		a.OnStart()
	}

	if e.execRouter != nil && len(e.accountIDs) > 0 {
		for _, acctID := range e.accountIDs {
			wt := e.walletTypes[acctID]
			if err := e.execRouter.ReqBalanceSnapshot(acctID, wt); err != nil {
				log().Error().Err(err).Int("accountID", acctID).Str("walletType", wt.String()).
					Msg("LedgerEngine: Failed to request balance snapshot")
			} else {
				log().Debug().Int("accountID", acctID).Str("walletType", wt.String()).
					Msg("LedgerEngine: Requested balance snapshot")
			}
		}
	} else {
		e.EngineBase.NotifyReady()
	}

	log().Info().Msg("LedgerEngine started")
}

// Stop stops the engine.
func (e *Engine) Stop() {
	for _, a := range e.actors {
		a.OnStop()
	}
	e.NotifyStop()
	log().Info().Msg("LedgerEngine stopped")
}

// ============================================================================
// balance.EngineHandler implementation
// ============================================================================

func (e *Engine) ResolveWallet(name string) (accountID int, walletID int, walletType common.WalletType, err error) {
	wallet, err := e.catalog.GetWalletByName(name)
	if err != nil {
		return 0, 0, common.WalletTypeUnknown, err
	}
	return wallet.AcctID, wallet.ID, wallet.WalletType, nil
}

// NotifyReady is called by each BalanceActor when its snapshot is received.
// When all balance actors have reported ready, the engine notifies the system.
func (e *Engine) NotifyReady() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.readyCount++
	if !e.ready && e.readyCount >= e.pendingCount {
		e.ready = true
		log().Info().Msg("LedgerEngine: All balance snapshots received, notifying ready")
		e.EngineBase.NotifyReady()
	}
}
