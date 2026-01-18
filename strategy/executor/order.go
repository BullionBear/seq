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
	Status        common.OrderStatus
	ExecutedQty   float64
	CreatedAt     uint64
	UpdatedAt     uint64
}
