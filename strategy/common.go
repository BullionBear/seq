package strategy

import (
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data/ob"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/portfolio"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// CacheService is the interface that Node's Cache implements.
// This interface is defined here to avoid circular imports.
// Note: Subscription and connection methods are removed - engines handle this via config.
type CacheService interface {
	// OrderBook queries
	GetBestBid(symbolID int) (price, qty float64, ok bool)
	GetBestAsk(symbolID int) (price, qty float64, ok bool)
	GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel)
	GetMidPrice(symbolID int) (price float64, ok bool)
	GetSpread(symbolID int) (spread float64, ok bool)
	IsSymbolReady(symbolID int) bool
	GetBookState(symbolID int) (ob.BookState, bool)

	// Execution queries
	GetOpenOrder(acctID int, clientOrderID int) *execution.OpenOrder
	GetOpenOrdersByAccount(acctID int) []*execution.OpenOrder
	GetOpenOrdersBySymbol(acctID int, symbolID int) []*execution.OpenOrder
	OpenOrderCount(acctID int) int

	// Portfolio queries
	GetBalance(acctID int, tokenID int) *portfolio.Balance
	GetAvailable(acctID int, tokenID int) float64
	GetLocked(acctID int, tokenID int) float64
	GetTotal(acctID int, tokenID int) float64
	GetAccountBalances(acctID int) *portfolio.AccountBalance
	HasSufficientBalance(acctID int, tokenID int, amount float64) bool

	// Order submission
	SubmitOrder(acctID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) (int, error)
	CancelOrder(acctID int, clientOrderID int) error
	CancelAllOrders(acctID int, symbolID int) error

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
// Order Management Methods (delegated to Execution Engine via Cache)
// ============================================================================

// SubmitLimitOrder submits a new limit order and returns the client order ID.
func (s *StrategyCommon) SubmitLimitOrder(acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) (int, error) {
	return s.cache.SubmitOrder(acctID, symbolID, side, common.OrderTypeLimit, timeInForce, price, quantity)
}

// SubmitMarketOrder submits a new market order and returns the client order ID.
func (s *StrategyCommon) SubmitMarketOrder(acctID int, symbolID int, side common.Side, quantity float64) (int, error) {
	return s.cache.SubmitOrder(acctID, symbolID, side, common.OrderTypeMarket, common.TimeInForceGTC, 0, quantity)
}

// CancelOrder cancels an existing order.
func (s *StrategyCommon) CancelOrder(acctID int, clientOrderID int) error {
	return s.cache.CancelOrder(acctID, clientOrderID)
}

// CancelAllOrders cancels all open orders for a symbol.
func (s *StrategyCommon) CancelAllOrders(acctID int, symbolID int) error {
	return s.cache.CancelAllOrders(acctID, symbolID)
}

// GetOpenOrder retrieves an open order by account ID and client order ID.
func (s *StrategyCommon) GetOpenOrder(acctID int, clientOrderID int) *execution.OpenOrder {
	return s.cache.GetOpenOrder(acctID, clientOrderID)
}

// GetOpenOrders returns all currently open orders for an account.
func (s *StrategyCommon) GetOpenOrders(acctID int) []*execution.OpenOrder {
	return s.cache.GetOpenOrdersByAccount(acctID)
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol.
func (s *StrategyCommon) GetOpenOrdersBySymbol(acctID int, symbolID int) []*execution.OpenOrder {
	return s.cache.GetOpenOrdersBySymbol(acctID, symbolID)
}

// OpenOrderCount returns the number of open orders for an account.
func (s *StrategyCommon) OpenOrderCount(acctID int) int {
	return s.cache.OpenOrderCount(acctID)
}

// ============================================================================
// Portfolio Methods (delegated to Portfolio Engine via Cache)
// ============================================================================

// GetBalance returns the balance for a specific account and token.
func (s *StrategyCommon) GetBalance(acctID int, tokenID int) *portfolio.Balance {
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
func (s *StrategyCommon) GetAccountBalances(acctID int) *portfolio.AccountBalance {
	return s.cache.GetAccountBalances(acctID)
}

// HasSufficientBalance checks if an account has sufficient available balance.
func (s *StrategyCommon) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	return s.cache.HasSufficientBalance(acctID, tokenID, amount)
}
