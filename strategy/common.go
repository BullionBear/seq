package strategy

import (
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// CacheService is the interface that Cache implements for read-only access.
// Write methods are not exposed here because strategies should not write to cache directly.
type CacheService interface {
	// OrderBook queries
	GetBestBid(symbolID int) (price, qty float64, ok bool)
	GetBestAsk(symbolID int) (price, qty float64, ok bool)
	GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel)
	GetMidPrice(symbolID int) (price float64, ok bool)
	GetSpread(symbolID int) (spread float64, ok bool)
	IsSymbolReady(symbolID int) bool
	GetBookState(symbolID int) (common.BookState, bool)

	// Execution queries
	GetOpenOrder(acctID int, clientOrderID int) *common.Order
	GetOpenOrdersByAccount(acctID int) []*common.Order
	GetOpenOrdersBySymbol(acctID int, symbolID int) []*common.Order
	OpenOrderCount(acctID int) int

	// Portfolio queries
	GetBalance(acctID int, tokenID int) *event.Balance
	GetAvailable(acctID int, tokenID int) float64
	GetLocked(acctID int, tokenID int) float64
	GetTotal(acctID int, tokenID int) float64
	GetAccountBalances(acctID int) map[int]*event.Balance
	HasSufficientBalance(acctID int, tokenID int, amount float64) bool
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
func (s *StrategyCommon) GetBookState(symbolID int) (common.BookState, bool) {
	return s.cache.GetBookState(symbolID)
}

// ============================================================================
// Order Query Methods (delegated to Cache)
// ============================================================================

// GetOpenOrder retrieves an open order by account ID and client order ID.
func (s *StrategyCommon) GetOpenOrder(acctID int, clientOrderID int) *common.Order {
	return s.cache.GetOpenOrder(acctID, clientOrderID)
}

// GetOpenOrders returns all currently open orders for an account.
func (s *StrategyCommon) GetOpenOrders(acctID int) []*common.Order {
	return s.cache.GetOpenOrdersByAccount(acctID)
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol.
func (s *StrategyCommon) GetOpenOrdersBySymbol(acctID int, symbolID int) []*common.Order {
	return s.cache.GetOpenOrdersBySymbol(acctID, symbolID)
}

// OpenOrderCount returns the number of open orders for an account.
func (s *StrategyCommon) OpenOrderCount(acctID int) int {
	return s.cache.OpenOrderCount(acctID)
}

// ============================================================================
// Portfolio Methods (delegated to Cache)
// ============================================================================

// GetBalance returns the balance for a specific account and token.
func (s *StrategyCommon) GetBalance(acctID int, tokenID int) *event.Balance {
	return s.cache.GetBalance(acctID, tokenID)
}

// GetAvailable returns the available balance for a specific account and token.
func (s *StrategyCommon) GetAvailable(acctID int, tokenID int) float64 {
	return s.cache.GetAvailable(acctID, tokenID)
}

// GetLocked returns the locked balance for a specific account and token.
func (s *StrategyCommon) GetLocked(acctID int, tokenID int) float64 {
	return s.cache.GetLocked(acctID, tokenID)
}

// GetTotalBalance returns the total balance for a specific account and token.
func (s *StrategyCommon) GetTotalBalance(acctID int, tokenID int) float64 {
	return s.cache.GetTotal(acctID, tokenID)
}

// GetAccountBalances returns all balances for an account.
func (s *StrategyCommon) GetAccountBalances(acctID int) map[int]*event.Balance {
	return s.cache.GetAccountBalances(acctID)
}

// HasSufficientBalance checks if an account has sufficient available balance.
func (s *StrategyCommon) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	return s.cache.HasSufficientBalance(acctID, tokenID, amount)
}
