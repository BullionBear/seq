package ob

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// OrderBookService provides orderbook query methods.
// Implemented by the OrderBook actor to provide market data access.
type OrderBookService interface {
	// GetBestBid returns the best bid price and quantity for the given symbol.
	// Returns ok=false if no bids are available.
	GetBestBid(symbolID int) (price, qty float64, ok bool)

	// GetBestAsk returns the best ask price and quantity for the given symbol.
	// Returns ok=false if no asks are available.
	GetBestAsk(symbolID int) (price, qty float64, ok bool)

	// GetDepth returns the top N levels of bids and asks for the given symbol.
	GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel)

	// GetMidPrice returns the mid price for the given symbol.
	// Returns ok=false if orderbook is empty on either side.
	GetMidPrice(symbolID int) (price float64, ok bool)

	// GetSpread returns the bid-ask spread for the given symbol.
	// Returns ok=false if orderbook is empty on either side.
	GetSpread(symbolID int) (spread float64, ok bool)
}

// LimitOrderRequest represents a request to submit a limit order.
type LimitOrderRequest struct {
	AccountID   int
	SymbolID    int
	Side        common.Side
	TimeInForce common.TimeInForce
	Quantity    float64
	Price       float64
}

// MarketOrderRequest represents a request to submit a market order.
type MarketOrderRequest struct {
	AccountID int
	SymbolID  int
	Side      common.Side
	Quantity  float64
}

// OrderService provides order management methods.
// Implemented by the EMS actor to handle order lifecycle.
type OrderService interface {
	// SubmitLimitOrder submits a new limit order.
	// Returns the client order ID assigned to this order.
	SubmitLimitOrder(req LimitOrderRequest) (clientOrderID int, err error)

	// SubmitMarketOrder submits a new market order.
	// Returns the client order ID assigned to this order.
	SubmitMarketOrder(req MarketOrderRequest) (clientOrderID int, err error)

	// CancelOrder requests cancellation of an existing order.
	CancelOrder(clientOrderID int) error

	// GetOrder retrieves an order by its client order ID.
	GetOrder(clientOrderID int) (common.Order, error)

	// GetOpenOrders returns all currently open orders.
	GetOpenOrders() []common.Order

	// GetOrdersBySymbol returns all orders for a specific symbol.
	GetOrdersBySymbol(symbolID int) []common.Order
}
