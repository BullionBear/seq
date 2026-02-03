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
type ExecutionClient interface {
	// Connect establishes the connection for trading
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect()

	// SubscribeUserDataStream subscribes to user data stream for account updates
	SubscribeUserDataStream() error

	// SubmitOrder submits a new order
	SubmitOrder(symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error

	// CancelOrder cancels an order by orderID
	CancelOrder(symbolID int, orderID int) error

	// CancelAllOrders cancels all open orders for a symbol
	CancelAllOrders(symbolID int) error
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
// Note: Order updates are published to the event bus by the execution client
func (r *ExecutionRouter) SubscribeOrderUpdate(acctID int) {
	// Order updates are automatically published to the event bus
	// by the execution client when received from the exchange
}

// SubscribeOrderFill subscribes to order fill events for an account
// Note: Fills are published to the event bus by the execution client
func (r *ExecutionRouter) SubscribeOrderFill(acctID int) {
	// Fills are automatically published to the event bus
	// by the execution client when received from the exchange
}

// Connect connects all registered execution clients and subscribes to user data streams
func (r *ExecutionRouter) Connect(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for acctID, client := range r.clients {
		// Connect the client
		if err := client.Connect(ctx); err != nil {
			return &RouterError{AccountID: acctID, Err: err}
		}

		// Subscribe to user data stream for account updates
		if err := client.SubscribeUserDataStream(); err != nil {
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
