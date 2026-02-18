package cache

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/alphadose/haxmap"
)

type OrderCache struct {
	orders              *haxmap.Map[int, *common.Order]                   // clientOrderID -> Order
	openOrdersByAccount *haxmap.Map[int, *haxmap.Map[int, *common.Order]] // accountID -> (clientOrderID -> Order)
}

func NewOrderCache() *OrderCache {
	return &OrderCache{
		orders:              haxmap.New[int, *common.Order](),
		openOrdersByAccount: haxmap.New[int, *haxmap.Map[int, *common.Order]](),
	}
}

func (oc *OrderCache) InsertOrder(order *common.Order) {
	oc.orders.Set(order.ClientOrderID, order)
	if order.OrderStatus.IsOpen() {
		oc.addOpenOrder(order)
	}
}

func (oc *OrderCache) UpdateOrder(order *common.Order) {
	prev, exists := oc.orders.Get(order.ClientOrderID)
	oc.orders.Set(order.ClientOrderID, order)

	wasOpen := exists && prev.OrderStatus.IsOpen()
	isOpen := order.OrderStatus.IsOpen()

	switch {
	case !wasOpen && isOpen:
		oc.addOpenOrder(order)
	case wasOpen && !isOpen:
		oc.removeOpenOrder(prev.AccountID, order.ClientOrderID)
	case wasOpen && isOpen:
		oc.addOpenOrder(order)
	}
}

func (oc *OrderCache) DeleteOrder(clientOrderID int) {
	order, ok := oc.orders.Get(clientOrderID)
	if !ok {
		return
	}
	oc.orders.Del(clientOrderID)
	if order.OrderStatus.IsOpen() {
		oc.removeOpenOrder(order.AccountID, clientOrderID)
	}
}

func (oc *OrderCache) GetOrder(clientOrderID int) *common.Order {
	order, ok := oc.orders.Get(clientOrderID)
	if !ok {
		return nil
	}
	return order
}

func (oc *OrderCache) GetOpenOrders(accountID int) []*common.Order {
	acctOrders, ok := oc.openOrdersByAccount.Get(accountID)
	if !ok {
		return nil
	}
	orders := make([]*common.Order, 0, acctOrders.Len())
	acctOrders.ForEach(func(_ int, order *common.Order) bool {
		orders = append(orders, order)
		return true
	})
	return orders
}

func (oc *OrderCache) addOpenOrder(order *common.Order) {
	acctOrders, _ := oc.openOrdersByAccount.GetOrCompute(order.AccountID, func() *haxmap.Map[int, *common.Order] {
		return haxmap.New[int, *common.Order]()
	})
	acctOrders.Set(order.ClientOrderID, order)
}

func (oc *OrderCache) removeOpenOrder(accountID, clientOrderID int) {
	if acctOrders, ok := oc.openOrdersByAccount.Get(accountID); ok {
		acctOrders.Del(clientOrderID)
	}
}
