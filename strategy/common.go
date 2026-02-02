package strategy

import (
	"context"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data/ob"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// CacheService is the interface that Node's Cache implements.
// This interface is defined here to avoid circular imports.
type CacheService interface {
	// OrderBook queries
	GetBestBid(symbolID int) (price, qty float64, ok bool)
	GetBestAsk(symbolID int) (price, qty float64, ok bool)
	GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel)
	GetMidPrice(symbolID int) (price float64, ok bool)
	GetSpread(symbolID int) (spread float64, ok bool)
	IsSymbolReady(symbolID int) bool
	GetBookState(symbolID int) (ob.BookState, bool)

	// Subscriptions
	SubscribeDepthUpdate(symbolID int)
	SubscribeTick(symbolID int)

	// Connections
	Connect(ctx context.Context)
	Disconnect()

	// Catalog access
	GetCatalog() interface{}
}

// StrategyCommon provides the common infrastructure for all strategies.
// It wraps the CacheService to provide a clean facade API for strategies.
type StrategyCommon struct {
	cache   CacheService
	catalog *catalog.Catalog
}

// NewStrategyCommon creates a new StrategyCommon with the given cache.
func NewStrategyCommon(cache CacheService, cat *catalog.Catalog) *StrategyCommon {
	return &StrategyCommon{
		cache:   cache,
		catalog: cat,
	}
}

// GetCatalog returns the catalog.
func (s *StrategyCommon) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// ============================================================================
// OrderBook Service Methods (delegated to Cache)
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (s *StrategyCommon) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	return s.cache.GetBestBid(symbolID)
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (s *StrategyCommon) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	return s.cache.GetBestAsk(symbolID)
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (s *StrategyCommon) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	return s.cache.GetDepth(symbolID, levels)
}

// GetMidPrice returns the mid price for the given symbol.
func (s *StrategyCommon) GetMidPrice(symbolID int) (price float64, ok bool) {
	return s.cache.GetMidPrice(symbolID)
}

// GetSpread returns the bid-ask spread for the given symbol.
func (s *StrategyCommon) GetSpread(symbolID int) (spread float64, ok bool) {
	return s.cache.GetSpread(symbolID)
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (s *StrategyCommon) IsSymbolReady(symbolID int) bool {
	return s.cache.IsSymbolReady(symbolID)
}

// GetBookState returns the state of the orderbook for a symbol.
func (s *StrategyCommon) GetBookState(symbolID int) (ob.BookState, bool) {
	return s.cache.GetBookState(symbolID)
}

// ============================================================================
// Order Management Methods (stub - will be delegated to Execution Engine)
// ============================================================================

// SubmitLimitOrder submits a new limit order and returns the client order ID.
func (s *StrategyCommon) SubmitLimitOrder(acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) int {
	// TODO: Delegate to ExecutionEngine via Cache
	return 0
}

// SubmitMarketOrder submits a new market order and returns the client order ID.
func (s *StrategyCommon) SubmitMarketOrder(acctID int, symbolID int, side common.Side, quantity float64) int {
	// TODO: Delegate to ExecutionEngine via Cache
	return 0
}

// CancelOrder cancels an existing order.
func (s *StrategyCommon) CancelOrder(clientOrderID int) error {
	// TODO: Delegate to ExecutionEngine via Cache
	return nil
}

// GetOrder retrieves an order by its client order ID.
func (s *StrategyCommon) GetOrder(clientOrderID int) (common.Order, error) {
	// TODO: Delegate to ExecutionEngine via Cache
	return common.Order{}, nil
}

// GetOpenOrders returns all currently open orders.
func (s *StrategyCommon) GetOpenOrders() []common.Order {
	// TODO: Delegate to ExecutionEngine via Cache
	return []common.Order{}
}

// GetOrdersBySymbol returns all orders for a specific symbol.
func (s *StrategyCommon) GetOrdersBySymbol(symbolID int) []common.Order {
	// TODO: Delegate to ExecutionEngine via Cache
	return []common.Order{}
}

// ============================================================================
// Private Subscription Methods (stub)
// ============================================================================

func (s *StrategyCommon) SubscribeOrderUpdate(acctID int) {
	// TODO: Delegate to ExecutionEngine via Cache
}

func (s *StrategyCommon) SubscribeBalanceUpdate(acctID int) {
	// TODO: Delegate to PortfolioEngine via Cache
}

func (s *StrategyCommon) SubscribeOrderFill(acctID int) {
	// TODO: Delegate to ExecutionEngine via Cache
}

// ============================================================================
// Public Subscription Methods (delegated to Cache)
// ============================================================================

func (s *StrategyCommon) SubscribeDepthUpdate(symbolID int) {
	s.cache.SubscribeDepthUpdate(symbolID)
}

func (s *StrategyCommon) SubscribeTick(symbolID int) {
	s.cache.SubscribeTick(symbolID)
}

// ============================================================================
// Operations Methods (delegated to Cache)
// ============================================================================

func (s *StrategyCommon) Connect(ctx context.Context) {
	s.cache.Connect(ctx)
}

func (s *StrategyCommon) Disconnect() {
	s.cache.Disconnect()
}
