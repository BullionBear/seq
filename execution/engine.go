package execution

import (
	"context"
	"sync"

	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// OpenOrder represents an open order with its current state
type OpenOrder struct {
	AccountID     int
	ClientOrderID int
	OrderID       int
	SymbolID      int
	Side          common.Side
	OrderType     common.OrderType
	TimeInForce   common.TimeInForce
	Quantity      float64
	Price         float64
	Status        common.OrderStatus
	ExecutedQty   float64
	CreatedAt     uint64
	UpdatedAt     uint64
}

// Engine manages order execution and maintains open order state
type Engine struct {
	router   *adapter.ExecutionRouter
	eventBus *evbus.EventBus

	// Configured account IDs for execution
	accountIDs []int

	// Open orders indexed by accountID -> clientOrderID -> OpenOrder
	openOrders map[int]map[int]*OpenOrder
	mu         sync.RWMutex

	// Client order ID generator per account
	nextClientOrderID map[int]int
	cidMu             sync.Mutex
}

// NewEngine creates a new execution engine
func NewEngine(router *adapter.ExecutionRouter, eventBus *evbus.EventBus) *Engine {
	return &Engine{
		router:            router,
		eventBus:          eventBus,
		openOrders:        make(map[int]map[int]*OpenOrder),
		nextClientOrderID: make(map[int]int),
	}
}

// Init initializes the execution engine
func (e *Engine) Init() {
	log().Debug().Msg("ExecutionEngine initialized")
}

// Start starts the execution engine
func (e *Engine) Start() {
	log().Debug().Msg("ExecutionEngine started")
}

// Stop stops the execution engine
func (e *Engine) Stop() {
	log().Debug().Msg("ExecutionEngine stopped")
}

// ============================================================================
// Config-based Account Setup
// ============================================================================

// SetAccounts configures account IDs for execution.
// Execution clients for these accounts should be registered with the router
// before calling Connect().
func (e *Engine) SetAccounts(accountIDs []int) {
	e.accountIDs = accountIDs
	log().Debug().Ints("accountIDs", accountIDs).Msg("ExecutionEngine: Configured accounts")
}

// GetConfiguredAccounts returns the configured account IDs.
func (e *Engine) GetConfiguredAccounts() []int {
	return e.accountIDs
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect connects the execution router
func (e *Engine) Connect(ctx context.Context) error {
	return e.router.Connect(ctx)
}

// Disconnect disconnects the execution router
func (e *Engine) Disconnect() {
	e.router.Disconnect()
}

// ============================================================================
// Order Submission
// ============================================================================

// SubmitOrder submits a new order and tracks it as an open order
func (e *Engine) SubmitOrder(acctID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) (int, error) {
	// Generate client order ID
	clientOrderID := e.generateClientOrderID(acctID)

	// Create open order entry
	order := &OpenOrder{
		AccountID:     acctID,
		ClientOrderID: clientOrderID,
		SymbolID:      symbolID,
		Side:          side,
		OrderType:     orderType,
		TimeInForce:   timeInForce,
		Quantity:      quantity,
		Price:         price,
		Status:        common.OrderStatusInFlight,
		ExecutedQty:   0,
	}

	// Add to open orders
	e.addOpenOrder(acctID, clientOrderID, order)

	// Submit to exchange via router
	err := e.router.SubmitOrder(acctID, symbolID, side, orderType, timeInForce, price, quantity)
	if err != nil {
		// Remove from open orders on failure
		e.removeOpenOrder(acctID, clientOrderID)
		return 0, err
	}

	log().Debug().
		Int("accountID", acctID).
		Int("clientOrderID", clientOrderID).
		Int("symbolID", symbolID).
		Str("side", sideToString(side)).
		Float64("price", price).
		Float64("quantity", quantity).
		Msg("Order submitted")

	return clientOrderID, nil
}

// CancelOrder cancels an order by client order ID
func (e *Engine) CancelOrder(acctID int, clientOrderID int) error {
	order := e.GetOpenOrder(acctID, clientOrderID)
	if order == nil {
		return ErrOrderNotFound
	}

	return e.router.CancelOrder(acctID, order.SymbolID, order.OrderID)
}

// CancelAllOrders cancels all open orders for a symbol
func (e *Engine) CancelAllOrders(acctID int, symbolID int) error {
	return e.router.CancelAllOrders(acctID, symbolID)
}

// ============================================================================
// Open Order Management
// ============================================================================

// GetOpenOrder returns an open order by account ID and client order ID
func (e *Engine) GetOpenOrder(acctID int, clientOrderID int) *OpenOrder {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctOrders, ok := e.openOrders[acctID]
	if !ok {
		return nil
	}
	return acctOrders[clientOrderID]
}

// GetOpenOrdersByAccount returns all open orders for an account
func (e *Engine) GetOpenOrdersByAccount(acctID int) []*OpenOrder {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctOrders, ok := e.openOrders[acctID]
	if !ok {
		return nil
	}

	orders := make([]*OpenOrder, 0, len(acctOrders))
	for _, order := range acctOrders {
		orders = append(orders, order)
	}
	return orders
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol
func (e *Engine) GetOpenOrdersBySymbol(acctID int, symbolID int) []*OpenOrder {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctOrders, ok := e.openOrders[acctID]
	if !ok {
		return nil
	}

	orders := make([]*OpenOrder, 0)
	for _, order := range acctOrders {
		if order.SymbolID == symbolID {
			orders = append(orders, order)
		}
	}
	return orders
}

// OpenOrderCount returns the number of open orders for an account
func (e *Engine) OpenOrderCount(acctID int) int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acctOrders, ok := e.openOrders[acctID]
	if !ok {
		return 0
	}
	return len(acctOrders)
}

// ============================================================================
// Event Handlers
// ============================================================================

// OnOrderAccepted handles OrderAccepted events
func (e *Engine) OnOrderAccepted(ev event.OrderAccepted) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find order by client order ID across all accounts
	order := e.findOrderByClientOrderID(ev.ClientOrderID)
	if order == nil {
		log().Warn().Int("clientOrderID", ev.ClientOrderID).Msg("OrderAccepted for unknown order")
		return
	}

	order.OrderID = ev.OrderID
	order.Status = common.OrderStatusAccepted
	order.CreatedAt = ev.CreatedAt
	order.UpdatedAt = ev.CreatedAt

	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Int("orderID", ev.OrderID).
		Msg("Order accepted")
}

// OnOrderPartiallyFilled handles OrderPartiallyFilled events
func (e *Engine) OnOrderPartiallyFilled(ev event.OrderPartiallyFilled) {
	e.mu.Lock()
	defer e.mu.Unlock()

	order := e.findOrderByClientOrderID(ev.ClientOrderID)
	if order == nil {
		log().Warn().Int("clientOrderID", ev.ClientOrderID).Msg("OrderPartiallyFilled for unknown order")
		return
	}

	order.Status = common.OrderStatusPartiallyFilled
	order.ExecutedQty = ev.ExecutedQty
	order.UpdatedAt = ev.UpdatedAt

	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Float64("executedQty", ev.ExecutedQty).
		Msg("Order partially filled")
}

// OnOrderFilled handles OrderFilled events
func (e *Engine) OnOrderFilled(ev event.OrderFilled) {
	e.mu.Lock()
	defer e.mu.Unlock()

	order := e.findOrderByClientOrderID(ev.ClientOrderID)
	if order == nil {
		log().Warn().Int("clientOrderID", ev.ClientOrderID).Msg("OrderFilled for unknown order")
		return
	}

	order.Status = common.OrderStatusFilled
	order.ExecutedQty = ev.ExecutedQty
	order.UpdatedAt = ev.UpdatedAt

	// Remove from open orders since it's fully filled
	e.removeOpenOrderUnsafe(order.AccountID, ev.ClientOrderID)

	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Float64("executedQty", ev.ExecutedQty).
		Msg("Order filled")
}

// OnOrderCanceled handles OrderCanceled events
func (e *Engine) OnOrderCanceled(ev event.OrderCanceled) {
	e.mu.Lock()
	defer e.mu.Unlock()

	order := e.findOrderByClientOrderID(ev.ClientOrderID)
	if order == nil {
		log().Warn().Int("clientOrderID", ev.ClientOrderID).Msg("OrderCanceled for unknown order")
		return
	}

	order.Status = common.OrderStatusCanceled
	order.UpdatedAt = ev.UpdatedAt

	// Remove from open orders
	e.removeOpenOrderUnsafe(order.AccountID, ev.ClientOrderID)

	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Msg("Order canceled")
}

// OnOrderRejected handles OrderRejected events
func (e *Engine) OnOrderRejected(ev event.OrderRejected) {
	e.mu.Lock()
	defer e.mu.Unlock()

	order := e.findOrderByClientOrderID(ev.ClientOrderID)
	if order == nil {
		log().Warn().Int("clientOrderID", ev.ClientOrderID).Msg("OrderRejected for unknown order")
		return
	}

	order.Status = common.OrderStatusRejected

	// Remove from open orders
	e.removeOpenOrderUnsafe(order.AccountID, ev.ClientOrderID)

	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Str("reason", ev.Msg).
		Msg("Order rejected")
}

// OnFill handles Fill events (for tracking fill details)
func (e *Engine) OnFill(ev event.Fill) {
	// Fill events are informational - the order state is updated by OnOrderPartiallyFilled/OnOrderFilled
	log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Int("fillID", ev.FillID).
		Float64("qty", ev.FilledQty).
		Float64("price", ev.FilledPrice).
		Msg("Fill received")
}

// ============================================================================
// Internal Methods
// ============================================================================

func (e *Engine) generateClientOrderID(acctID int) int {
	e.cidMu.Lock()
	defer e.cidMu.Unlock()

	e.nextClientOrderID[acctID]++
	return e.nextClientOrderID[acctID]
}

func (e *Engine) addOpenOrder(acctID int, clientOrderID int, order *OpenOrder) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.openOrders[acctID]; !ok {
		e.openOrders[acctID] = make(map[int]*OpenOrder)
	}
	e.openOrders[acctID][clientOrderID] = order
}

func (e *Engine) removeOpenOrder(acctID int, clientOrderID int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeOpenOrderUnsafe(acctID, clientOrderID)
}

func (e *Engine) removeOpenOrderUnsafe(acctID int, clientOrderID int) {
	if acctOrders, ok := e.openOrders[acctID]; ok {
		delete(acctOrders, clientOrderID)
	}
}

// findOrderByClientOrderID finds an order by client order ID across all accounts
// Must be called with lock held
func (e *Engine) findOrderByClientOrderID(clientOrderID int) *OpenOrder {
	for _, acctOrders := range e.openOrders {
		if order, ok := acctOrders[clientOrderID]; ok {
			return order
		}
	}
	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func sideToString(s common.Side) string {
	switch s {
	case common.SideBuy:
		return "BUY"
	case common.SideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// Errors
// ============================================================================

var ErrOrderNotFound = &OrderError{Msg: "order not found"}

type OrderError struct {
	Msg string
}

func (e *OrderError) Error() string {
	return e.Msg
}
