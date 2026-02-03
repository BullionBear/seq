package portfolio

import (
	"sync"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure Engine implements the Actor interface
var _ actor.Actor = (*Engine)(nil)

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

// Engine manages portfolio state including balances for each account
type Engine struct {
	eventBus *evbus.EventBus

	// Configured account IDs to track
	accountIDs []int

	// Balances indexed by accountID -> tokenID -> Balance
	balances map[int]*AccountBalance
	mu       sync.RWMutex
}

// NewEngine creates a new portfolio engine
func NewEngine(eventBus *evbus.EventBus) *Engine {
	return &Engine{
		eventBus:   eventBus,
		accountIDs: make([]int, 0),
		balances:   make(map[int]*AccountBalance),
	}
}

// ============================================================================
// Actor Interface Implementation
// ============================================================================

// Name returns the unique identifier for this actor
func (e *Engine) Name() string {
	return "PortfolioEngine"
}

// SubscribedTypes returns the event types this engine wants to receive
func (e *Engine) SubscribedTypes() []event.Topic {
	return []event.Topic{
		event.TopicEventBalanceUpdate,
		event.TopicEventReqBalanceSnapshot,
		event.TopicEventFill,
	}
}

// Handle processes events from the event bus
func (e *Engine) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeBalanceUpdate(buf)
		e.OnBalanceUpdate(update)
	case event.TopicEventReqBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqBalanceSnapshot(buf)
		e.OnReqBalanceSnapshot(snapshot)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		e.OnFill(fill)
	}
}

// OnInit is called once when the actor is initialized
func (e *Engine) OnInit() {
	log().Debug().Msg("PortfolioEngine initialized")
}

// OnStart is called once when the actor is started
func (e *Engine) OnStart() {
	log().Debug().Msg("PortfolioEngine started")
}

// OnStop is called once when the actor is stopped
func (e *Engine) OnStop() {
	log().Debug().Msg("PortfolioEngine stopped")
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

// OnReqBalanceSnapshot handles ReqBalanceSnapshot events from the event bus
func (e *Engine) OnReqBalanceSnapshot(ev event.ReqBalanceSnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	log().Debug().
		Int("accountID", ev.AccountID).
		Int("balanceCount", len(ev.Balances)).
		Msg("Balance snapshot received")
}

// OnFill handles Fill events to update balances based on fills
// This provides real-time balance updates based on trade executions
func (e *Engine) OnFill(ev event.Fill) {
	// Note: Fill events can be used to adjust balances in real-time
	// However, the primary source of truth is BalanceUpdate events
	// This is kept for informational purposes
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
