package event

import (
	"github.com/BullionBear/seq/core/model/common"
)

type OrderInitialized struct {
	StrategyID    int
	ClientOrderID int
	AccountID     int
	OrderStatus   common.OrderStatus
	SymbolID      int
	Side          common.Side
	Quantity      float64
	Price         float64
	OrderType     common.OrderType
	TimeInForce   common.TimeInForce
	ExecutedQty   float64
	UpdatedAt     uint64
}

type OrderUpdate struct {
	ClientOrderID int
	OrderID       int
	OrderStatus   common.OrderStatus
	ExecutedQty   float64
	UpdatedAt     uint64
}

type Fill struct {
	ClientOrderID int
	OrderID       int
	FillID        int
	FilledQty     float64
	FilledPrice   float64
	FeeCcyID      int
	FeeQty        float64
	FilledAt      uint64
}
