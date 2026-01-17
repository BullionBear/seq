package integration

type PriceLevel struct {
	Price    float64
	Quantity float64
}

type DepthResponse struct {
	SymbolID int
	DepthID  int
	Asks     []PriceLevel
	Bids     []PriceLevel
}

type CreateOrderRequest struct {
	SymbolID    int
	Side        Side
	Type        OrderType
	TimeInForce TimeInForce
	Price       float64
	Quantity    float64
}

type CreateOrderResponse struct {
	OrderID int
}
