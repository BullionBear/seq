package portfolio

import (
	"sync"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// BalanceConfigurer is an optional interface that portfolio actors can implement
// to receive the execution router and account IDs from the engine.
type BalanceConfigurer interface {
	Configure(router *adapter.ExecutionRouter, accountIDs []int)
}

// Balance represents the balance for a specific token
type Balance struct {
	TokenID   int
	Available float64
	Locked    float64
	Total     float64
}

// AccountBalance represents all balances for an account
type AccountBalance struct {
	AccountID int
	Balances  map[int]*Balance // tokenID -> Balance
	UpdatedAt uint64
}

// Engine manages portfolio state including balances for each account.
// It constructs actors from config via the factory registry.
type Engine struct {
	engine.EngineBase
	msgBus *msgbus.MsgBus
	actors []actor.Actor

	// Execution router for subscribing and requesting balance snapshots
	execRouter *adapter.ExecutionRouter

	// Configured account IDs to track
	accountIDs []int

	// Balances indexed by accountID -> tokenID -> Balance
	balances map[int]*AccountBalance
	mu       sync.RWMutex
}

// NewEngine creates a new portfolio engine
func NewEngine(msgBus *msgbus.MsgBus) *Engine {
	return &Engine{
		EngineBase: engine.NewEngineBase(common.EnginePortfolio),
		msgBus:     msgBus,
		accountIDs: make([]int, 0),
		balances:   make(map[int]*AccountBalance),
	}
}

// SetExecutionRouter sets the execution router for subscribing and requesting balance snapshots
func (e *Engine) SetExecutionRouter(router *adapter.ExecutionRouter) {
	e.execRouter = router
}

// SetAccounts sets the account IDs to track for portfolio management
func (e *Engine) SetAccounts(accountIDs []int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.accountIDs = accountIDs

	for _, acctID := range accountIDs {
		e.ensureAccountExistsLocked(acctID)
	}

	log().Info().
		Ints("accountIDs", accountIDs).
		Msg("PortfolioEngine: Configured accounts")
}

// ============================================================================
// Engine Interface Implementation
// ============================================================================

// Init constructs actors from config, configures them, and registers with the EventBus.
func (e *Engine) Init(config Config) {
	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			log().Error().Err(err).Str("type", entry.Type).Msg("PortfolioEngine: skipping unknown actor type")
			continue
		}

		a := factory(e)
		actor.ApplyName(a, entry.Name)
		// If the actor supports balance configuration, pass router and accounts
		if bc, ok := a.(BalanceConfigurer); ok {
			bc.Configure(e.execRouter, e.accountIDs)
		}
		actor.Register(e.msgBus, a)
		a.OnInit(entry.Config)
		e.actors = append(e.actors, a)

		log().Info().Str("type", entry.Type).Str("name", a.Name()).Msg("PortfolioEngine: actor initialized")
	}

	if e.execRouter == nil {
		log().Warn().Msg("PortfolioEngine: No execution router configured, skipping balance subscription")
	} else {
		for _, acctID := range e.accountIDs {
			if err := e.execRouter.SubscribeBalance(acctID); err != nil {
				log().Error().Err(err).Int("accountID", acctID).Msg("PortfolioEngine: Failed to subscribe to balance updates")
			} else {
				log().Debug().Int("accountID", acctID).Msg("PortfolioEngine: Subscribed to balance updates")
			}
		}
	}

	log().Info().Msg("PortfolioEngine initialized")
}

// Start triggers actors to start (e.g. request initial balance snapshots).
func (e *Engine) Start() {
	for _, a := range e.actors {
		a.OnStart()
	}
	log().Info().Msg("PortfolioEngine started")
}

// Stop stops the engine.
func (e *Engine) Stop() {
	for _, a := range e.actors {
		a.OnStop()
	}
	e.NotifyStop()
	log().Info().Msg("PortfolioEngine stopped")
}

// ============================================================================
// Configuration Access
// ============================================================================

// GetConfiguredAccounts returns the list of configured account IDs
func (e *Engine) GetConfiguredAccounts() []int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.accountIDs
}

// ============================================================================
// Balance Access
// ============================================================================

func (e *Engine) GetBalance(acctID int, tokenID int) *Balance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acctBalance, ok := e.balances[acctID]
	if !ok {
		return nil
	}
	return acctBalance.Balances[tokenID]
}

func (e *Engine) GetAvailable(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Available
}

func (e *Engine) GetLocked(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Locked
}

func (e *Engine) GetTotal(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Total
}

func (e *Engine) GetAccountBalances(acctID int) *AccountBalance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.balances[acctID]
}

func (e *Engine) GetAllTokenBalances(acctID int) []*Balance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acctBalance, ok := e.balances[acctID]
	if !ok {
		return nil
	}
	balances := make([]*Balance, 0, len(acctBalance.Balances))
	for _, b := range acctBalance.Balances {
		balances = append(balances, b)
	}
	return balances
}

func (e *Engine) GetNonZeroBalances(acctID int) []*Balance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acctBalance, ok := e.balances[acctID]
	if !ok {
		return nil
	}
	balances := make([]*Balance, 0)
	for _, b := range acctBalance.Balances {
		if b.Total > 0 {
			balances = append(balances, b)
		}
	}
	return balances
}

func (e *Engine) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	return e.GetAvailable(acctID, tokenID) >= amount
}

// ============================================================================
// Balance Updates
// ============================================================================

func (e *Engine) SetBalance(acctID int, tokenID int, available float64, locked float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureAccountExistsLocked(acctID)
	e.balances[acctID].Balances[tokenID] = &Balance{
		TokenID:   tokenID,
		Available: available,
		Locked:    locked,
		Total:     available + locked,
	}
}

func (e *Engine) UpdateBalance(acctID int, balance *Balance, updatedAt uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureAccountExistsLocked(acctID)
	e.balances[acctID].Balances[balance.TokenID] = balance
	e.balances[acctID].UpdatedAt = updatedAt
}

// ============================================================================
// Event Handlers (implements BalanceEngineHandler)
// ============================================================================

func (e *Engine) OnBalanceUpdate(ev event.BalanceUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureAccountExistsLocked(ev.AccountID)
	for _, b := range ev.Balances {
		e.balances[ev.AccountID].Balances[b.TokenID] = &Balance{
			TokenID:   b.TokenID,
			Available: b.Available,
			Locked:    b.Locked,
			Total:     b.Total,
		}
	}
	e.balances[ev.AccountID].UpdatedAt = ev.UpdatedAt
	log().Debug().
		Int("accountID", ev.AccountID).
		Int("balanceCount", len(ev.Balances)).
		Msg("Balance updated")
}

func (e *Engine) OnRespBalanceSnapshot(ev event.RespBalanceSnapshot) {
	e.mu.Lock()
	e.ensureAccountExistsLocked(ev.AccountID)
	e.balances[ev.AccountID].Balances = make(map[int]*Balance)
	for _, b := range ev.Balances {
		e.balances[ev.AccountID].Balances[b.TokenID] = &Balance{
			TokenID:   b.TokenID,
			Available: b.Available,
			Locked:    b.Locked,
			Total:     b.Total,
		}
	}
	e.mu.Unlock()

	log().Info().
		Int("accountID", ev.AccountID).
		Int("balanceCount", len(ev.Balances)).
		Msg("PortfolioEngine: Balance snapshot received and initialized")

	for _, b := range ev.Balances {
		if b.Total > 0 {
			log().Debug().
				Int("accountID", ev.AccountID).
				Int("tokenID", b.TokenID).
				Float64("available", b.Available).
				Float64("locked", b.Locked).
				Float64("total", b.Total).
				Msg("PortfolioEngine: Token balance initialized")
		}
	}
}

func (e *Engine) OnExecution(ev event.Execution) {
	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Int("fillID", ev.FillID).
		Float64("qty", ev.FilledQty).
		Float64("price", ev.FilledPrice).
		Float64("fee", ev.FeeQty).
		Msg("Fill received in portfolio engine")
}

// ============================================================================
// Internal Methods
// ============================================================================

func (e *Engine) ensureAccountExistsLocked(acctID int) {
	if _, ok := e.balances[acctID]; !ok {
		e.balances[acctID] = &AccountBalance{
			AccountID: acctID,
			Balances:  make(map[int]*Balance),
		}
	}
}

func (e *Engine) ClearAccount(acctID int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.balances, acctID)
}

func (e *Engine) AccountCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.balances)
}

func (e *Engine) TokenCount(acctID int) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	acctBalance, ok := e.balances[acctID]
	if !ok {
		return 0
	}
	return len(acctBalance.Balances)
}
