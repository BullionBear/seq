package command

import "github.com/BullionBear/seq/core/model/common"

type SubmitOrder struct {
	AccountID   int
	SymbolID    int
	Side        common.Side
	OrderType   common.OrderType
	TimeInForce common.TimeInForce
	Price       float64
	Quantity    float64
}

type CancelOrder struct {
	AccountID     int
	ClientOrderID int
}

type CancelAll struct {
	AccountID int
	SymbolID  int
}
