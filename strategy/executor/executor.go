package executor

import (
	"sync"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/internal/evbus"
)

type Executor struct {
	ActiveOrders     map[int]Order
	eventBus         *evbus.EventBus
	OrderArena       sync.Pool
	gateway          OrderGateway
	clientOrderIDRef int
}

func NewExecutor() *Executor {
	return &Executor{
		ActiveOrders: make(map[int]Order),
		OrderArena: sync.Pool{
			New: func() interface{} {
				return &Order{}
			},
		},
		gateway:          OrderGateway{},
		clientOrderIDRef: 0,
	}
}

func (e *Executor) SubmitLimitOrder(strategyID int, acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) {
	order := e.OrderArena.Get().(*Order)
	order.Reset()
	order.StrategyID = strategyID
	order.AccountID = acctID
	order.SymbolID = symbolID
	order.Side = side
	order.TimeInForce = timeInForce
	order.Quantity = quantity
	order.Price = price
	order.OrderType = common.OrderTypeLimit

	// WARNING: strict assumption that SubmitOrder does not retain pointer ownership
	e.gateway.SubmitOrder(order)
	e.OrderArena.Put(order)
}

func (e *Executor) SubmitMarketOrder(strategyID int, acctID int, symbolID int, side common.Side, quantity float64) {
	order := e.OrderArena.Get().(*Order)
	order.Reset()
	order.StrategyID = strategyID
	order.AccountID = acctID
	order.SymbolID = symbolID
	order.Side = side
	order.Quantity = quantity
	order.OrderType = common.OrderTypeMarket

	// WARNING: strict assumption that SubmitOrder does not retain pointer ownership
	e.gateway.SubmitOrder(order)
	e.OrderArena.Put(order)
}
