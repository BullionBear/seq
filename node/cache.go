package node

import (
	"context"

	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/data/ob"
)

// Cache provides a facade for strategies to access data from various engines.
// It abstracts the underlying engine implementations and provides a clean API.
// Cache implements strategy.CacheService interface.
type Cache struct {
	dataEngine *data.Engine
	// Future: riskEngine, portfolioEngine, executionEngine
}

// NewCache creates a new Cache with the given data engine.
func NewCache(dataEngine *data.Engine) *Cache {
	return &Cache{
		dataEngine: dataEngine,
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
// Connection Methods (delegated to DataEngine)
// ============================================================================

// Connect connects to all data sources.
func (c *Cache) Connect(ctx context.Context) {
	c.dataEngine.Connect(ctx)
}

// Disconnect disconnects from all data sources.
func (c *Cache) Disconnect() {
	c.dataEngine.Disconnect()
}

// ============================================================================
// Catalog Access
// ============================================================================

// GetCatalog returns the catalog from the data engine.
func (c *Cache) GetCatalog() interface{} {
	return c.dataEngine.Catalog()
}
