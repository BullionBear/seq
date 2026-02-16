package portfolio

import (
	"sync"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	pactor "github.com/BullionBear/seq/portfolio/actor"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

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
// It is NOT an actor — it owns a BalanceActor that handles EventBus events.
type Engine struct {
	engine.EngineBase
	msgBus       *msgbus.MsgBus
	balanceActor *pactor.BalanceActor

	// Execution router for subscribing and requesting balance snapshots
	execRouter *adapter.ExecutionRouter

	// Configured account IDs to track
	accountIDs []int

	// Balances indexed by accountID -> tokenID -> Balance
	balances map[int]*AccountBalance
	mu       sync.RWMutex
}

// NewEngine creates a new portfolio engine
func NewEngine(msgBus *msgbus.MsgBus, cache *cache.Cache) *Engine {
	e := &Engine{
		EngineBase: engine.NewEngineBase(common.EnginePortfolio),
		msgBus:     msgBus,
		accountIDs: make([]int, 0),
		balances:   make(map[int]*AccountBalance),
	}
	e.balanceActor = pactor.NewBalanceActor(e)
	return e
}

// SetExecutionRouter sets the execution router for subscribing and requesting balance snapshots
func (e *Engine) SetExecutionRouter(router *adapter.ExecutionRouter) {
	e.execRouter = router
}

// ============================================================================
// Engine Interface Implementation
// ============================================================================

// Init configures the BalanceActor, registers it with the EventBus, and subscribes
// to ongoing balance updates for all configured accounts.
func (e *Engine) Init() {
	e.balanceActor.Configure(e.execRouter, e.accountIDs)
	actor.Register(e.msgBus, e.balanceActor)
	if e.execRouter == nil {
		log().Warn().Msg("PortfolioEngine: No execution router configured, skipping balance subscription")
		return
	}

	// Subscribe to balance updates for all configured accounts
	for _, acctID := range e.accountIDs {
		if err := e.execRouter.SubscribeBalance(acctID); err != nil {
			log().Error().Err(err).Int("accountID", acctID).Msg("PortfolioEngine: Failed to subscribe to balance updates")
		} else {
			log().Debug().Int("accountID", acctID).Msg("PortfolioEngine: Subscribed to balance updates")
		}
	}

	log().Info().Msg("PortfolioEngine initialized")
}

// Start triggers the BalanceActor to request initial balance snapshots.
// The actor calls NotifyReady once all snapshots are received.
func (e *Engine) Start() {
	e.balanceActor.OnStart()
	log().Info().Msg("PortfolioEngine started")
}

// Stop stops the engine.
func (e *Engine) Stop() {
	e.NotifyStop()
	log().Info().Msg("PortfolioEngine stopped")
}

// ============================================================================
// Configuration
// ============================================================================

// SetAccounts sets the account IDs to track for portfolio management
func (e *Engine) SetAccounts(accountIDs []int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.accountIDs = accountIDs

	// Pre-initialize account balances
	for _, acctID := range accountIDs {
		e.ensureAccountExistsLocked(acctID)
	}

	log().Info().
		Ints("accountIDs", accountIDs).
		Msg("PortfolioEngine: Configured accounts")
}

// GetConfiguredAccounts returns the list of configured account IDs
func (e *Engine) GetConfiguredAccounts() []int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.accountIDs
}

// ============================================================================
// Balance Access
// ============================================================================

// GetBalance returns the balance for a specific account and token
func (e *Engine) GetBalance(acctID int, tokenID int) *Balance {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctBalance, ok := e.balances[acctID]
	if !ok {
		return nil
	}
	return acctBalance.Balances[tokenID]
}

// GetAvailable returns the available balance for a specific account and token
func (e *Engine) GetAvailable(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Available
}

// GetLocked returns the locked balance for a specific account and token
func (e *Engine) GetLocked(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Locked
}

// GetTotal returns the total balance for a specific account and token
func (e *Engine) GetTotal(acctID int, tokenID int) float64 {
	balance := e.GetBalance(acctID, tokenID)
	if balance == nil {
		return 0
	}
	return balance.Total
}

// GetAccountBalances returns all balances for an account
func (e *Engine) GetAccountBalances(acctID int) *AccountBalance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.balances[acctID]
}

// GetAllTokenBalances returns all balances for a specific account as a slice
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

// GetNonZeroBalances returns all non-zero balances for an account
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

// HasSufficientBalance checks if an account has sufficient available balance
func (e *Engine) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	return e.GetAvailable(acctID, tokenID) >= amount
}

// ============================================================================
// Balance Updates
// ============================================================================

// SetBalance sets the balance for a specific account and token
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

// UpdateBalance updates the balance for a specific account and token
func (e *Engine) UpdateBalance(acctID int, balance *Balance, updatedAt uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ensureAccountExistsLocked(acctID)

	e.balances[acctID].Balances[balance.TokenID] = balance
	e.balances[acctID].UpdatedAt = updatedAt
}

// ============================================================================
// Event Handlers
// ============================================================================

// OnBalanceUpdate handles BalanceUpdate events from the event bus
func (e *Engine) OnBalanceUpdate(ev event.BalanceUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ensureAccountExistsLocked(ev.AccountID)

	// Update all balances from the event
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

// OnReqBalanceSnapshot handles ReqBalanceSnapshot events from the event bus.
// Balance state is updated here; the BalanceActor tracks completion and calls NotifyReady.
func (e *Engine) OnRespBalanceSnapshot(ev event.RespBalanceSnapshot) {
	e.mu.Lock()
	e.ensureAccountExistsLocked(ev.AccountID)

	// Clear existing balances and set from snapshot
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

// OnFill handles Fill events to update balances based on fills
func (e *Engine) OnFill(ev event.Fill) {
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

// ensureAccountExistsLocked creates account balance entry if not exists.
// Must be called while holding the lock.
func (e *Engine) ensureAccountExistsLocked(acctID int) {
	if _, ok := e.balances[acctID]; !ok {
		e.balances[acctID] = &AccountBalance{
			AccountID: acctID,
			Balances:  make(map[int]*Balance),
		}
	}
}

// ============================================================================
// Utility Methods
// ============================================================================

// ClearAccount clears all balances for an account
func (e *Engine) ClearAccount(acctID int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.balances, acctID)
}

// AccountCount returns the number of accounts being tracked
func (e *Engine) AccountCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.balances)
}

// TokenCount returns the number of tokens being tracked for an account
func (e *Engine) TokenCount(acctID int) int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctBalance, ok := e.balances[acctID]
	if !ok {
		return 0
	}
	return len(acctBalance.Balances)
}
