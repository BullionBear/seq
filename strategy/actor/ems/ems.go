package ems

import (
	"errors"
	"sync"
	"time"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy/actor"
)

// Ensure EMS implements the Actor interface
var _ actor.Actor = (*EMS)(nil)

// Ensure EMS implements the OrderService interface
var _ actor.OrderService = (*EMS)(nil)

var (
	ErrOrderNotFound = errors.New("order not found")
)

// EMS (Execution Management System) is an actor that manages order lifecycle.
type EMS struct {
	actor.ActorBase
	mu             sync.RWMutex
	orders         map[int]*common.Order // clientOrderID -> order
	ordersBySymbol map[int][]int         // symbolID -> []clientOrderID
	nextOrderID    int
	eventBus       *evbus.EventBus
}

// NewEMS creates a new EMS actor.
func NewEMS() *EMS {
	return &EMS{
		ActorBase: actor.NewActorBase("ems", []event.DataType{
			event.DataTypeOrderUpdate,
			event.DataTypeFill,
		}),
		orders:         make(map[int]*common.Order),
		ordersBySymbol: make(map[int][]int),
		nextOrderID:    1,
	}
}

// SetEventBus sets the event bus reference for publishing order events.
func (ems *EMS) SetEventBus(bus *evbus.EventBus) {
	ems.eventBus = bus
}

// Handle processes order-related events.
func (ems *EMS) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.DataType {
	case event.DataTypeOrderUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderUpdate := evbus.DeserializeOrderUpdate(buf)
		ems.onOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		ems.onFill(fill)
	}
}

func (ems *EMS) onOrderUpdate(orderUpdate event.OrderUpdate) {
	ems.mu.Lock()
	defer ems.mu.Unlock()

	order, exists := ems.orders[orderUpdate.ClientOrderID]
	if !exists {
		return
	}

	order.OrderStatus = orderUpdate.OrderStatus
	order.ExecutedQty = orderUpdate.ExecutedQty
	order.UpdatedAt = orderUpdate.UpdatedAt
}

func (ems *EMS) onFill(fill event.Fill) {
	ems.mu.Lock()
	defer ems.mu.Unlock()

	order, exists := ems.orders[fill.ClientOrderID]
	if !exists {
		return
	}

	order.ExecutedQty += fill.FilledQty
	order.UpdatedAt = fill.FilledAt

	// Check if fully filled
	if order.ExecutedQty >= order.Quantity {
		order.OrderStatus = common.OrderStatusFilled
	} else {
		order.OrderStatus = common.OrderStatusPartiallyFilled
	}
}

// SubmitLimitOrder submits a new limit order.
func (ems *EMS) SubmitLimitOrder(req actor.LimitOrderRequest) (clientOrderID int, err error) {
	ems.mu.Lock()
	defer ems.mu.Unlock()

	clientOrderID = ems.nextOrderID
	ems.nextOrderID++

	now := uint64(time.Now().UnixNano())
	order := &common.Order{
		AccountID:     req.AccountID,
		ClientOrderID: clientOrderID,
		SymbolID:      req.SymbolID,
		Side:          req.Side,
		OrderType:     common.OrderTypeLimit,
		TimeInForce:   req.TimeInForce,
		Quantity:      req.Quantity,
		Price:         req.Price,
		OrderStatus:   common.OrderStatusInitialized,
		ExecutedQty:   0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	ems.orders[clientOrderID] = order
	ems.ordersBySymbol[req.SymbolID] = append(ems.ordersBySymbol[req.SymbolID], clientOrderID)

	// TODO: Send order to exchange via execution router
	// For now, just mark as in-flight
	order.OrderStatus = common.OrderStatusInFlight

	return clientOrderID, nil
}

// SubmitMarketOrder submits a new market order.
func (ems *EMS) SubmitMarketOrder(req actor.MarketOrderRequest) (clientOrderID int, err error) {
	ems.mu.Lock()
	defer ems.mu.Unlock()

	clientOrderID = ems.nextOrderID
	ems.nextOrderID++

	now := uint64(time.Now().UnixNano())
	order := &common.Order{
		AccountID:     req.AccountID,
		ClientOrderID: clientOrderID,
		SymbolID:      req.SymbolID,
		Side:          req.Side,
		OrderType:     common.OrderTypeMarket,
		TimeInForce:   common.TimeInForceIOC,
		Quantity:      req.Quantity,
		Price:         0, // Market order has no price
		OrderStatus:   common.OrderStatusInitialized,
		ExecutedQty:   0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	ems.orders[clientOrderID] = order
	ems.ordersBySymbol[req.SymbolID] = append(ems.ordersBySymbol[req.SymbolID], clientOrderID)

	// TODO: Send order to exchange via execution router
	order.OrderStatus = common.OrderStatusInFlight

	return clientOrderID, nil
}

// CancelOrder requests cancellation of an existing order.
func (ems *EMS) CancelOrder(clientOrderID int) error {
	ems.mu.Lock()
	defer ems.mu.Unlock()

	order, exists := ems.orders[clientOrderID]
	if !exists {
		return ErrOrderNotFound
	}

	// Check if order can be cancelled
	if order.OrderStatus == common.OrderStatusFilled ||
		order.OrderStatus == common.OrderStatusCanceled ||
		order.OrderStatus == common.OrderStatusRejected {
		return errors.New("order cannot be cancelled")
	}

	// TODO: Send cancel request to exchange via execution router
	// For now, just mark as cancelled
	order.OrderStatus = common.OrderStatusCanceled
	order.UpdatedAt = uint64(time.Now().UnixNano())

	return nil
}

// GetOrder retrieves an order by its client order ID.
func (ems *EMS) GetOrder(clientOrderID int) (common.Order, error) {
	ems.mu.RLock()
	defer ems.mu.RUnlock()

	order, exists := ems.orders[clientOrderID]
	if !exists {
		return common.Order{}, ErrOrderNotFound
	}
	return *order, nil
}

// GetOpenOrders returns all currently open orders.
func (ems *EMS) GetOpenOrders() []common.Order {
	ems.mu.RLock()
	defer ems.mu.RUnlock()

	var openOrders []common.Order
	for _, order := range ems.orders {
		if isOpenOrder(order.OrderStatus) {
			openOrders = append(openOrders, *order)
		}
	}
	return openOrders
}

// GetOrdersBySymbol returns all orders for a specific symbol.
func (ems *EMS) GetOrdersBySymbol(symbolID int) []common.Order {
	ems.mu.RLock()
	defer ems.mu.RUnlock()

	orderIDs, exists := ems.ordersBySymbol[symbolID]
	if !exists {
		return nil
	}

	var orders []common.Order
	for _, id := range orderIDs {
		if order, ok := ems.orders[id]; ok {
			orders = append(orders, *order)
		}
	}
	return orders
}

// isOpenOrder returns true if the order status indicates an open order.
func isOpenOrder(status common.OrderStatus) bool {
	return status == common.OrderStatusInitialized ||
		status == common.OrderStatusInFlight ||
		status == common.OrderStatusAccepted ||
		status == common.OrderStatusPartiallyFilled
}
