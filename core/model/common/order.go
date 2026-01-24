package common

type Order struct {
	AccountID     int
	ClientOrderID int
	SymbolID      int
	Side          Side
	OrderType     OrderType
	TimeInForce   TimeInForce
	Quantity      float64
	Price         float64
	OrderStatus   OrderStatus
	ExecutedQty   float64
	CreatedAt     uint64
	UpdatedAt     uint64
}
