package cache

import (
	"sync"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// bookData holds the orderbook state for a single symbol.
type bookData struct {
	bids  []event.PriceLevel
	asks  []event.PriceLevel
	state common.BookState
}

// Cache maintains a self-contained data store for strategies to read.
// Engines write data into the cache; strategies read from it.
// Cache has no dependency on any engine package.
type Cache struct {
	mu sync.RWMutex

	// Orderbook data: symbolID -> bookData
	books map[int]*bookData

	// Open orders: acctID -> clientOrderID -> Order
	openOrders map[int]map[int]*common.Order

	// Balances: acctID -> tokenID -> Balance
	balances map[int]map[int]*event.Balance
}

// NewCache creates a new empty Cache.
func NewCache() *Cache {
	return &Cache{
		books:      make(map[int]*bookData),
		openOrders: make(map[int]map[int]*common.Order),
		balances:   make(map[int]map[int]*event.Balance),
	}
}

// ============================================================================
// OrderBook Read Methods
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (c *Cache) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists || book.state != common.BookStateReady || len(book.bids) == 0 {
		return 0, 0, false
	}
	return book.bids[0].Price, book.bids[0].Quantity, true
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (c *Cache) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists || book.state != common.BookStateReady || len(book.asks) == 0 {
		return 0, 0, false
	}
	return book.asks[0].Price, book.asks[0].Quantity, true
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (c *Cache) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists {
		return nil, nil
	}

	bidN := levels
	if bidN > len(book.bids) {
		bidN = len(book.bids)
	}
	askN := levels
	if askN > len(book.asks) {
		askN = len(book.asks)
	}

	bidsCopy := make([]event.PriceLevel, bidN)
	copy(bidsCopy, book.bids[:bidN])
	asksCopy := make([]event.PriceLevel, askN)
	copy(asksCopy, book.asks[:askN])

	return bidsCopy, asksCopy
}

// GetMidPrice returns the mid price for the given symbol.
func (c *Cache) GetMidPrice(symbolID int) (price float64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists || book.state != common.BookStateReady || len(book.bids) == 0 || len(book.asks) == 0 {
		return 0, false
	}
	return (book.bids[0].Price + book.asks[0].Price) / 2, true
}

// GetSpread returns the bid-ask spread for the given symbol.
func (c *Cache) GetSpread(symbolID int) (spread float64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists || book.state != common.BookStateReady || len(book.bids) == 0 || len(book.asks) == 0 {
		return 0, false
	}
	return book.asks[0].Price - book.bids[0].Price, true
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (c *Cache) IsSymbolReady(symbolID int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists {
		return false
	}
	return book.state == common.BookStateReady
}

// GetBookState returns the state of the orderbook for a symbol.
func (c *Cache) GetBookState(symbolID int) (common.BookState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	book, exists := c.books[symbolID]
	if !exists {
		return common.BookStateWaitForSnapshot, false
	}
	return book.state, true
}

// ============================================================================
// OrderBook Write Methods
// ============================================================================

// UpdateBook updates the orderbook depth for a symbol.
// bids and asks should be sorted (best first).
func (c *Cache) UpdateBook(symbolID int, bids, asks []event.PriceLevel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	book := c.ensureBook(symbolID)
	book.bids = bids
	book.asks = asks
}

// SetBookState sets the synchronization state of the orderbook for a symbol.
func (c *Cache) SetBookState(symbolID int, state common.BookState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	book := c.ensureBook(symbolID)
	book.state = state
}

// ============================================================================
// Open Order Read Methods
// ============================================================================

// GetOpenOrder returns an open order by account ID and client order ID.
func (c *Cache) GetOpenOrder(acctID int, clientOrderID int) *common.Order {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acctOrders, ok := c.openOrders[acctID]
	if !ok {
		return nil
	}
	return acctOrders[clientOrderID]
}

// GetOpenOrdersByAccount returns all open orders for an account.
func (c *Cache) GetOpenOrdersByAccount(acctID int) []*common.Order {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acctOrders, ok := c.openOrders[acctID]
	if !ok {
		return nil
	}

	orders := make([]*common.Order, 0, len(acctOrders))
	for _, order := range acctOrders {
		orders = append(orders, order)
	}
	return orders
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol.
func (c *Cache) GetOpenOrdersBySymbol(acctID int, symbolID int) []*common.Order {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acctOrders, ok := c.openOrders[acctID]
	if !ok {
		return nil
	}

	orders := make([]*common.Order, 0)
	for _, order := range acctOrders {
		if order.SymbolID == symbolID {
			orders = append(orders, order)
		}
	}
	return orders
}

// OpenOrderCount returns the number of open orders for an account.
func (c *Cache) OpenOrderCount(acctID int) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acctOrders, ok := c.openOrders[acctID]
	if !ok {
		return 0
	}
	return len(acctOrders)
}

// ============================================================================
// Open Order Write Methods
// ============================================================================

// SetOpenOrder adds or updates an open order in the cache.
func (c *Cache) SetOpenOrder(acctID int, order *common.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.openOrders[acctID]; !ok {
		c.openOrders[acctID] = make(map[int]*common.Order)
	}
	c.openOrders[acctID][order.ClientOrderID] = order
}

// RemoveOpenOrder removes an open order from the cache.
func (c *Cache) RemoveOpenOrder(acctID int, clientOrderID int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if acctOrders, ok := c.openOrders[acctID]; ok {
		delete(acctOrders, clientOrderID)
	}
}

// ============================================================================
// Balance Read Methods
// ============================================================================

// GetBalance returns the balance for a specific account and token.
func (c *Cache) GetBalance(acctID int, tokenID int) *event.Balance {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acctBalances, ok := c.balances[acctID]
	if !ok {
		return nil
	}
	return acctBalances[tokenID]
}

// GetAvailable returns the available balance for a specific account and token.
func (c *Cache) GetAvailable(acctID int, tokenID int) float64 {
	b := c.GetBalance(acctID, tokenID)
	if b == nil {
		return 0
	}
	return b.Available
}

// GetLocked returns the locked balance for a specific account and token.
func (c *Cache) GetLocked(acctID int, tokenID int) float64 {
	b := c.GetBalance(acctID, tokenID)
	if b == nil {
		return 0
	}
	return b.Locked
}

// GetTotal returns the total balance for a specific account and token.
func (c *Cache) GetTotal(acctID int, tokenID int) float64 {
	b := c.GetBalance(acctID, tokenID)
	if b == nil {
		return 0
	}
	return b.Total
}

// GetAccountBalances returns all balances for an account as a map of tokenID to Balance.
func (c *Cache) GetAccountBalances(acctID int) map[int]*event.Balance {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.balances[acctID]
}

// HasSufficientBalance checks if an account has sufficient available balance.
func (c *Cache) HasSufficientBalance(acctID int, tokenID int, amount float64) bool {
	return c.GetAvailable(acctID, tokenID) >= amount
}

// ============================================================================
// Balance Write Methods
// ============================================================================

// SetBalance sets the balance for a specific account and token.
func (c *Cache) SetBalance(acctID int, tokenID int, available, locked, total float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureAccount(acctID)
	c.balances[acctID][tokenID] = &event.Balance{
		TokenID:   tokenID,
		Available: available,
		Locked:    locked,
		Total:     total,
	}
}

// SetAccountBalances replaces all balances for an account.
func (c *Cache) SetAccountBalances(acctID int, balances []event.Balance) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.balances[acctID] = make(map[int]*event.Balance, len(balances))
	for i := range balances {
		b := balances[i]
		c.balances[acctID][b.TokenID] = &b
	}
}

// ============================================================================
// Internal Helpers
// ============================================================================

func (c *Cache) ensureBook(symbolID int) *bookData {
	book, exists := c.books[symbolID]
	if !exists {
		book = &bookData{
			state: common.BookStateWaitForSnapshot,
		}
		c.books[symbolID] = book
	}
	return book
}

func (c *Cache) ensureAccount(acctID int) {
	if _, ok := c.balances[acctID]; !ok {
		c.balances[acctID] = make(map[int]*event.Balance)
	}
}
