package executor

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

type OrderManager struct {
	OrderStatusMap   map[int]common.OrderStatus
	AllOrders        map[int]Order
	InitializedOrder map[int]struct{}
	InFlightOrder    map[int]struct{}
	ActiveOrder      map[int]struct{}
	CompletedOrder   map[int]struct{}
	CancelledOrder   map[int]struct{}
	RejectedOrder    map[int]struct{}
}

func NewOrderManager() *OrderManager {
	return &OrderManager{
		OrderStatusMap:   make(map[int]common.OrderStatus, 1024),
		AllOrders:        make(map[int]Order, 1024),
		InitializedOrder: make(map[int]struct{}, 1024),
		InFlightOrder:    make(map[int]struct{}, 1024),
		ActiveOrder:      make(map[int]struct{}, 1024),
		CompletedOrder:   make(map[int]struct{}, 1024),
		CancelledOrder:   make(map[int]struct{}, 1024),
		RejectedOrder:    make(map[int]struct{}, 1024),
	}
}

func (om *OrderManager) OnOrderInitialized(orderInitialized *event.OrderInitialized) {
	om.OrderStatusMap[orderInitialized.ClientOrderID] = common.OrderStatusInitialized
	om.AllOrders[orderInitialized.ClientOrderID] = Order{
		StrategyID:    orderInitialized.StrategyID,
		AccountID:     orderInitialized.AccountID,
		ClientOrderID: orderInitialized.ClientOrderID,
		OrderStatus:   common.OrderStatusInitialized,
		SymbolID:      orderInitialized.SymbolID,
		Side:          orderInitialized.Side,
		Quantity:      orderInitialized.Quantity,
		Price:         orderInitialized.Price,
		OrderType:     orderInitialized.OrderType,
		TimeInForce:   orderInitialized.TimeInForce,
		ExecutedQty:   0,
		CreatedAt:     orderInitialized.UpdatedAt,
		UpdatedAt:     orderInitialized.UpdatedAt,
	}
	om.InitializedOrder[orderInitialized.ClientOrderID] = struct{}{}
}

func (om *OrderManager) OnOrderUpdate(orderUpdate event.OrderUpdate) {
	om.updateOrder(orderUpdate.ClientOrderID, orderUpdate)
}

func (om *OrderManager) removeOrder(clientOrderID int) {
	switch om.OrderStatusMap[clientOrderID] {
	case common.OrderStatusInFlight:
		delete(om.InFlightOrder, clientOrderID)
	case common.OrderStatusAccepted:
		delete(om.ActiveOrder, clientOrderID)
	case common.OrderStatusPartiallyFilled:
		delete(om.ActiveOrder, clientOrderID)
	case common.OrderStatusFilled:
		delete(om.CompletedOrder, clientOrderID)
	case common.OrderStatusCanceled:
		delete(om.CancelledOrder, clientOrderID)
	case common.OrderStatusRejected:
		delete(om.RejectedOrder, clientOrderID)
	}
}

func (om *OrderManager) addOrder(clientOrderID int) {
	switch om.OrderStatusMap[clientOrderID] {
	case common.OrderStatusInFlight:
		om.InFlightOrder[clientOrderID] = struct{}{}
	case common.OrderStatusAccepted:
		om.ActiveOrder[clientOrderID] = struct{}{}
	case common.OrderStatusPartiallyFilled:
		om.ActiveOrder[clientOrderID] = struct{}{}
	case common.OrderStatusFilled:
		om.CompletedOrder[clientOrderID] = struct{}{}
	case common.OrderStatusCanceled:
		om.CancelledOrder[clientOrderID] = struct{}{}
	case common.OrderStatusRejected:
		om.RejectedOrder[clientOrderID] = struct{}{}
	}
}

func (om *OrderManager) updateOrder(clientOrderID int, orderUpdate event.OrderUpdate) {
	// prevStatus := om.OrderStatusMap[clientOrderID].OrderStatus
	om.removeOrder(clientOrderID)
	order := om.AllOrders[clientOrderID]
	order.OrderStatus = orderUpdate.OrderStatus
	order.OrderID = orderUpdate.OrderID
	order.ExecutedQty = orderUpdate.ExecutedQty
	order.UpdatedAt = orderUpdate.UpdatedAt
	om.AllOrders[clientOrderID] = order
	om.OrderStatusMap[clientOrderID] = orderUpdate.OrderStatus
	om.addOrder(clientOrderID)
}
