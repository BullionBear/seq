package executor

import (
	"github.com/BullionBear/seq/core/model/common"
)

type Order struct {
	StrategyID    int
	AccountID     int
	OrderID       int
	ClientOrderID int
	SymbolID      int
	OrderType     common.OrderType
	Side          common.Side
	Quantity      float64
	Price         float64
	TimeInForce   common.TimeInForce
	OrderStatus   common.OrderStatus
	ExecutedQty   float64
	CreatedAt     uint64
	UpdatedAt     uint64
}

func (o *Order) Reset() {
	o.StrategyID = 0
	o.AccountID = 0
	o.OrderID = 0
	o.ClientOrderID = 0
	o.SymbolID = 0
	o.OrderType = 0
	o.Side = 0
	o.Quantity = 0
	o.Price = 0
	o.TimeInForce = 0
	o.OrderStatus = common.OrderStatusInitialized
	o.ExecutedQty = 0
	o.CreatedAt = 0
	o.UpdatedAt = 0
}
