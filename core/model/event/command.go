package event

import "github.com/BullionBear/seq/core/model/common"

// OrderSubmitCommand is the command payload for submitting a new order.
type OrderSubmitCommand struct {
	AccountID   int
	SymbolID    int
	Side        common.Side
	OrderType   common.OrderType
	TimeInForce common.TimeInForce
	Price       float64
	Quantity    float64
}

// OrderCancelCommand is the command payload for cancelling an order.
type OrderCancelCommand struct {
	AccountID     int
	ClientOrderID int
}

// CancelAllCommand is the command payload for cancelling all orders for a symbol.
type CancelAllCommand struct {
	AccountID int
	SymbolID  int
}
