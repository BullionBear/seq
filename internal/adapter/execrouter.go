package adapter

import (
	"context"
	"errors"
	"sync"

	"github.com/BullionBear/seq/core/model/common"
)

var (
	ErrAccountNotFound = errors.New("account not found in execution router")
	ErrClientNotFound  = errors.New("execution client not found for account")
)

// ExecutionClient is the interface that execution clients must implement.
// Each execution client handles order operations for a specific account.
//
// Subscription Model:
// The interface provides granular subscription methods for different private data types.
// Each exchange handles these differently internally:
//   - Bybit: Subscribes to individual topics (order, execution, wallet) as requested
//   - Binance: Subscribes to entire user data stream on first subscribe call,
//     then filters events internally based on which subscriptions are active
//
// Unsubscribed events are logged as "unhandled" and not published to the event bus.
type ExecutionClient interface {
	// Connect establishes the connection for trading
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect()

	// SubscribeOrderUpdate subscribes to order status update events
	// Events: OrderAccepted, OrderPartiallyFilled, OrderFilled, OrderCanceled, OrderRejected
	SubscribeOrderUpdate() error

	// SubscribeFill subscribes to execution/fill events
	// Events: Fill (trade execution details including price, quantity, fees)
	SubscribeFill() error

	// SubscribeBalance subscribes to wallet/balance update events
	// Events: BalanceUpdate (available, locked, total for each asset)
	SubscribeBalance() error

	// SubmitOrder submits a new order
	SubmitOrder(symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error

	// CancelOrder cancels an order by orderID
	CancelOrder(symbolID int, orderID int) error

	// CancelAllOrders cancels all open orders for a symbol
	CancelAllOrders(symbolID int) error

	// ReqBalanceSnapshot requests the current balance snapshot
	// The response will be published as a ReqBalanceSnapshot event
	ReqBalanceSnapshot() error
}

// ExecutionRouter routes order operations to the appropriate ExecutionClient
// based on account ID. It manages a collection of execution clients and
// provides a unified interface for order management.
type ExecutionRouter struct {
	clients map[int]ExecutionClient
	mu      sync.RWMutex
}

// NewExecutionRouter creates a new ExecutionRouter
func NewExecutionRouter() *ExecutionRouter {
	return &ExecutionRouter{
		clients: make(map[int]ExecutionClient),
	}
}

// RegisterClient registers an ExecutionClient for a specific account ID
func (r *ExecutionRouter) RegisterClient(acctID int, client ExecutionClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[acctID] = client
}

// UnregisterClient removes an ExecutionClient for a specific account ID
func (r *ExecutionRouter) UnregisterClient(acctID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, acctID)
}

// GetClient returns the ExecutionClient for a specific account ID
func (r *ExecutionRouter) GetClient(acctID int) (ExecutionClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[acctID]
	if !ok {
		return nil, ErrClientNotFound
	}
	return client, nil
}

// SubmitOrder routes the order submission to the appropriate client
func (r *ExecutionRouter) SubmitOrder(acctID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.SubmitOrder(symbolID, side, orderType, timeInForce, price, quantity)
}

// CancelOrder routes the order cancellation to the appropriate client
func (r *ExecutionRouter) CancelOrder(acctID int, symbolID int, orderID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.CancelOrder(symbolID, orderID)
}

// CancelAllOrders routes the cancel all orders request to the appropriate client
func (r *ExecutionRouter) CancelAllOrders(acctID int, symbolID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.CancelAllOrders(symbolID)
}

// SubscribeOrderUpdate subscribes to order update events for an account
func (r *ExecutionRouter) SubscribeOrderUpdate(acctID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.SubscribeOrderUpdate()
}

// SubscribeFill subscribes to fill/execution events for an account
func (r *ExecutionRouter) SubscribeFill(acctID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.SubscribeFill()
}

// SubscribeBalance subscribes to balance update events for an account
func (r *ExecutionRouter) SubscribeBalance(acctID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.SubscribeBalance()
}

// ReqBalanceSnapshot requests the current balance snapshot for an account
func (r *ExecutionRouter) ReqBalanceSnapshot(acctID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.ReqBalanceSnapshot()
}

// Connect connects all registered execution clients
// Note: Subscriptions must be called separately after Connect
func (r *ExecutionRouter) Connect(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for acctID, client := range r.clients {
		if err := client.Connect(ctx); err != nil {
			return &RouterError{AccountID: acctID, Err: err}
		}
	}
	return nil
}

// ConnectAccount connects a specific account's execution client
func (r *ExecutionRouter) ConnectAccount(ctx context.Context, acctID int) error {
	client, err := r.GetClient(acctID)
	if err != nil {
		return err
	}
	return client.Connect(ctx)
}

// Disconnect disconnects all registered execution clients
func (r *ExecutionRouter) Disconnect() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		client.Disconnect()
	}
}

// DisconnectAccount disconnects a specific account's execution client
func (r *ExecutionRouter) DisconnectAccount(acctID int) {
	client, err := r.GetClient(acctID)
	if err != nil {
		return
	}
	client.Disconnect()
}

// ClientCount returns the number of registered execution clients
func (r *ExecutionRouter) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// RouterError represents an error that occurred for a specific account
type RouterError struct {
	AccountID int
	Err       error
}

func (e *RouterError) Error() string {
	return "execution router error for account " + string(rune(e.AccountID)) + ": " + e.Err.Error()
}

func (e *RouterError) Unwrap() error {
	return e.Err
}
