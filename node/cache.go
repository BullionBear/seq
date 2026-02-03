package node

import (
	"context"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/data/ob"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/portfolio"
)

// Cache provides a facade for strategies to access data from various engines.
// It abstracts the underlying engine implementations and provides a clean API.
// Cache implements strategy.CacheService interface.
type Cache struct {
	dataEngine      *data.Engine
	executionEngine *execution.Engine
	portfolioEngine *portfolio.Engine
}

// NewCache creates a new Cache with the given engines.
func NewCache(dataEngine *data.Engine, executionEngine *execution.Engine, portfolioEngine *portfolio.Engine) *Cache {
	return &Cache{
		dataEngine:      dataEngine,
		executionEngine: executionEngine,
		portfolioEngine: portfolioEngine,
	}
}

// ============================================================================
// OrderBook Query Methods (delegated to DataEngine)
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (c *Cache) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	return c.dataEngine.GetBestBid(symbolID)
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (c *Cache) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	return c.dataEngine.GetBestAsk(symbolID)
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (c *Cache) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	return c.dataEngine.GetDepth(symbolID, levels)
}

// GetMidPrice returns the mid price for the given symbol.
func (c *Cache) GetMidPrice(symbolID int) (price float64, ok bool) {
	return c.dataEngine.GetMidPrice(symbolID)
}

// GetSpread returns the bid-ask spread for the given symbol.
func (c *Cache) GetSpread(symbolID int) (spread float64, ok bool) {
	return c.dataEngine.GetSpread(symbolID)
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (c *Cache) IsSymbolReady(symbolID int) bool {
	return c.dataEngine.IsSymbolReady(symbolID)
}

// GetBookState returns the state of the orderbook for a symbol.
func (c *Cache) GetBookState(symbolID int) (ob.BookState, bool) {
	return c.dataEngine.GetBookState(symbolID)
}

// ============================================================================
// Execution Query Methods (delegated to ExecutionEngine)
// ============================================================================

// GetOpenOrder returns an open order by account ID and client order ID.
func (c *Cache) GetOpenOrder(acctID int, clientOrderID int) *execution.OpenOrder {
	if c.executionEngine == nil {
		return nil
	}
	return c.executionEngine.GetOpenOrder(acctID, clientOrderID)
}

// GetOpenOrdersByAccount returns all open orders for an account.
func (c *Cache) GetOpenOrdersByAccount(acctID int) []*execution.OpenOrder {
	if c.executionEngine == nil {
		return nil
	}
	return c.executionEngine.GetOpenOrdersByAccount(acctID)
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol.
func (c *Cache) GetOpenOrdersBySymbol(acctID int, symbolID int) []*execution.OpenOrder {
	if c.executionEngine == nil {
		return nil
	}
	return c.executionEngine.GetOpenOrdersBySymbol(acctID, symbolID)
}

// OpenOrderCount returns the number of open orders for an account.
func (c *Cache) OpenOrderCount(acctID int) int {
	if c.executionEngine == nil {
		return 0
	}
	return c.executionEngine.OpenOrderCount(acctID)
}

// ============================================================================
// Portfolio Query Methods (delegated to PortfolioEngine)
// ============================================================================

// GetBalance returns the balance for a specific account and token.
func (c *Cache) GetBalance(acctID int, tokenID int) *portfolio.Balance {
	if c.portfolioEngine == nil {
		return nil
	}
	return c.portfolioEngine.GetBalance(acctID, tokenID)
}

// GetAvailable returns the available balance for a specific account and token.
func (c *Cache) GetAvailable(acctID int, tokenID int) float64 {
	if c.portfolioEngine == nil {
		return 0
	}
	return c.portfolioEngine.GetAvailable(acctID, tokenID)
}

// GetLocked returns the locked balance for a specific account and token.
func (c *Cache) GetLocked(acctID int, tokenID int) float64 {
	if c.portfolioEngine == nil {
		return 0
	}
	return c.portfolioEngine.GetLocked(acctID, tokenID)
}

// GetTotal returns the total balance for a specific account and token.
func (c *Cache) GetTotal(acctID int, tokenID int) float64 {
	if c.portfolioEngine == nil {
		return 0
	}
	return c.portfolioEngine.GetTotal(acctID, tokenID)
}

// GetAccountBalances returns all balances for an account.
func (c *Cache) GetAccountBalances(acctID int) *portfolio.AccountBalance {
	if c.portfolioEngine == nil {
		return nil
	}
	return c.portfolioEngine.GetAccountBalances(acctID)
}

// HasSufficientBalance checks if an account has sufficient available balance.
func (c *Cache) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	if c.portfolioEngine == nil {
		return false
	}
	return c.portfolioEngine.HasSufficientBalance(acctID, tokenID, amount)
}

// ============================================================================
// Order Submission Methods (delegated to ExecutionEngine)
// ============================================================================

// SubmitOrder submits a new order and returns the client order ID.
func (c *Cache) SubmitOrder(acctID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) (int, error) {
	if c.executionEngine == nil {
		return 0, &CacheError{Msg: "execution engine not available"}
	}
	return c.executionEngine.SubmitOrder(acctID, symbolID, side, orderType, timeInForce, price, quantity)
}

// CancelOrder cancels an order by client order ID.
func (c *Cache) CancelOrder(acctID int, clientOrderID int) error {
	if c.executionEngine == nil {
		return &CacheError{Msg: "execution engine not available"}
	}
	return c.executionEngine.CancelOrder(acctID, clientOrderID)
}

// CancelAllOrders cancels all open orders for a symbol.
func (c *Cache) CancelAllOrders(acctID int, symbolID int) error {
	if c.executionEngine == nil {
		return &CacheError{Msg: "execution engine not available"}
	}
	return c.executionEngine.CancelAllOrders(acctID, symbolID)
}

// ============================================================================
// Subscription Methods (delegated to DataEngine)
// ============================================================================

// SubscribeDepthUpdate subscribes to depth updates for a symbol.
func (c *Cache) SubscribeDepthUpdate(symbolID int) {
	c.dataEngine.SubscribeDepthUpdate(symbolID)
}

// SubscribeTick subscribes to tick updates for a symbol.
func (c *Cache) SubscribeTick(symbolID int) {
	c.dataEngine.SubscribeTick(symbolID)
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect connects to all data sources and execution.
func (c *Cache) Connect(ctx context.Context) {
	c.dataEngine.Connect(ctx)
	if c.executionEngine != nil {
		_ = c.executionEngine.Connect(ctx)
	}
}

// Disconnect disconnects from all data sources and execution.
func (c *Cache) Disconnect() {
	c.dataEngine.Disconnect()
	if c.executionEngine != nil {
		c.executionEngine.Disconnect()
	}
}

// ============================================================================
// Catalog Access
// ============================================================================

// GetCatalog returns the catalog from the data engine.
func (c *Cache) GetCatalog() interface{} {
	return c.dataEngine.Catalog()
}

// ============================================================================
// Engine Access (for direct engine operations if needed)
// ============================================================================

// ExecutionEngine returns the execution engine.
func (c *Cache) ExecutionEngine() *execution.Engine {
	return c.executionEngine
}

// PortfolioEngine returns the portfolio engine.
func (c *Cache) PortfolioEngine() *portfolio.Engine {
	return c.portfolioEngine
}

// DataEngine returns the data engine.
func (c *Cache) DataEngine() *data.Engine {
	return c.dataEngine
}

// ============================================================================
// Errors
// ============================================================================

type CacheError struct {
	Msg string
}

func (e *CacheError) Error() string {
	return e.Msg
}
