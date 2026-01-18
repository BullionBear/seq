package executor

import (
	"sync"

	"github.com/BullionBear/seq/core/model/common"
)

type Executor struct {
	ActiveOrders map[int]Order
	OrderArena   sync.Pool
	gateway      OrderGateway
}

func NewExecutor() *Executor {
	return &Executor{
		ActiveOrders: make(map[int]Order),
		OrderArena: sync.Pool{
			New: func() interface{} {
				return &Order{}
			},
		},
	}
}

func (e *Executor) SubmitLimitOrder(strategyID int, acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) *Order {
	order := e.OrderArena.Get().(*Order)
	order.StrategyID = strategyID
	order.AccountID = acctID
	order.SymbolID = symbolID
	order.Side = side
	order.TimeInForce = timeInForce
	order.Quantity = quantity
	order.Price = price
	return order
}

func (e *Executor) ExecuteOrder(order Order) error {
	return nil
}
