package cache

import (
	"github.com/alphadose/haxmap"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/tidwall/btree"
)

// OrderBook holds the orderbook state for a single symbol.
// Uses PriceTick as btree key; QuantityTick==0 indicates a deleted level.
type OrderBook struct {
	bids  btree.Map[int, common.PriceLevel] // key = PriceTick
	asks  btree.Map[int, common.PriceLevel] // key = PriceTick
	state common.BookState
}

// Cache maintains a self-contained data store for strategies to read.
// Engines write data into the cache; strategies read from it.
// Cache has no dependency on any engine package.
type Cache struct {
	// Orderbook data: symbolID -> OrderBook
	books *haxmap.Map[int, *OrderBook]

	// Order tracking with open-orders index
	orderCache *OrderCache

	// Balances: acctID -> (tokenID -> Balance)
	balances *haxmap.Map[int, *haxmap.Map[int, *common.Balance]]
}

// NewCache creates a new empty Cache.
func NewCache() *Cache {
	return &Cache{
		books:      haxmap.New[int, *OrderBook](),
		orderCache: NewOrderCache(),
		balances:   haxmap.New[int, *haxmap.Map[int, *common.Balance]](),
	}
}

// ============================================================================
// OrderBook Read Methods
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (c *Cache) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	book, exists := c.books.Get(symbolID)
	if !exists || book.state != common.BookStateReady || book.bids.Len() == 0 {
		return 0, 0, false
	}
	var best common.PriceLevel
	book.bids.Reverse(func(key int, value common.PriceLevel) bool {
		best = value
		return false // stop after first (highest bid)
	})
	return best.Price, best.Quantity, true
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (c *Cache) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	book, exists := c.books.Get(symbolID)
	if !exists || book.state != common.BookStateReady || book.asks.Len() == 0 {
		return 0, 0, false
	}
	var best common.PriceLevel
	book.asks.Scan(func(key int, value common.PriceLevel) bool {
		best = value
		return false // stop after first (lowest ask)
	})
	return best.Price, best.Quantity, true
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (c *Cache) GetDepth(symbolID int, levels int) (bids, asks []common.PriceLevel) {
	book, exists := c.books.Get(symbolID)
	if !exists {
		return nil, nil
	}

	var bidsCopy, asksCopy []common.PriceLevel
	book.bids.Reverse(func(key int, value common.PriceLevel) bool {
		if len(bidsCopy) < levels {
			bidsCopy = append(bidsCopy, value)
		}
		return len(bidsCopy) < levels
	})
	book.asks.Scan(func(key int, value common.PriceLevel) bool {
		if len(asksCopy) < levels {
			asksCopy = append(asksCopy, value)
		}
		return len(asksCopy) < levels
	})
	return bidsCopy, asksCopy
}

// GetMidPrice returns the mid price for the given symbol.
func (c *Cache) GetMidPrice(symbolID int) (price float64, ok bool) {
	bidPrice, _, bidOk := c.GetBestBid(symbolID)
	askPrice, _, askOk := c.GetBestAsk(symbolID)
	if !bidOk || !askOk {
		return 0, false
	}
	return (bidPrice + askPrice) / 2, true
}

// GetSpread returns the bid-ask spread for the given symbol.
func (c *Cache) GetSpread(symbolID int) (spread float64, ok bool) {
	bidPrice, _, bidOk := c.GetBestBid(symbolID)
	askPrice, _, askOk := c.GetBestAsk(symbolID)
	if !bidOk || !askOk {
		return 0, false
	}
	return askPrice - bidPrice, true
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (c *Cache) IsSymbolReady(symbolID int) bool {
	book, exists := c.books.Get(symbolID)
	if !exists {
		return false
	}
	return book.state == common.BookStateReady
}

// GetBookState returns the state of the orderbook for a symbol.
func (c *Cache) GetBookState(symbolID int) (common.BookState, bool) {
	book, exists := c.books.Get(symbolID)
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
// QuantityTick==0 indicates the level should be deleted (removed from the book).
func (c *Cache) UpdateBook(symbolID int, bids, asks []common.PriceLevel) {
	book := c.ensureBook(symbolID)
	// Clear and rebuild from incoming levels
	book.bids = btree.Map[int, common.PriceLevel]{}
	book.asks = btree.Map[int, common.PriceLevel]{}
	for _, pl := range bids {
		if pl.QuantityTick != 0 {
			book.bids.Set(pl.PriceTick, pl)
		} else {
			book.bids.Delete(pl.PriceTick)
		}
	}
	for _, pl := range asks {
		if pl.QuantityTick != 0 {
			book.asks.Set(pl.PriceTick, pl)
		} else {
			book.asks.Delete(pl.PriceTick)
		}
	}
}

// ApplyBookUpdate applies incremental depth updates to the orderbook for a symbol.
// Unlike UpdateBook, this does NOT clear the existing book; it applies deltas.
// QuantityTick==0 indicates the level should be deleted (removed from the book).
func (c *Cache) ApplyBookUpdate(symbolID int, bids, asks []common.PriceLevel) {
	book := c.ensureBook(symbolID)
	for _, pl := range bids {
		if pl.QuantityTick != 0 {
			book.bids.Set(pl.PriceTick, pl)
		} else {
			book.bids.Delete(pl.PriceTick)
		}
	}
	for _, pl := range asks {
		if pl.QuantityTick != 0 {
			book.asks.Set(pl.PriceTick, pl)
		} else {
			book.asks.Delete(pl.PriceTick)
		}
	}
}

// SetBookState sets the synchronization state of the orderbook for a symbol.
func (c *Cache) SetBookState(symbolID int, state common.BookState) {
	book := c.ensureBook(symbolID)
	book.state = state
}

// ============================================================================
// Order Read Methods
// ============================================================================

// GetOrder returns an order by client order ID, or nil if not found.
func (c *Cache) GetOrder(clientOrderID int) *common.Order {
	return c.orderCache.GetOrder(clientOrderID)
}

// GetOpenOrder returns an open order by account ID and client order ID.
func (c *Cache) GetOpenOrder(acctID int, clientOrderID int) *common.Order {
	order := c.orderCache.GetOrder(clientOrderID)
	if order == nil || order.AccountID != acctID || !order.OrderStatus.IsOpen() {
		return nil
	}
	return order
}

// GetOpenOrdersByAccount returns all open orders for an account.
func (c *Cache) GetOpenOrdersByAccount(acctID int) []*common.Order {
	return c.orderCache.GetOpenOrders(acctID)
}

// GetOpenOrdersBySymbol returns all open orders for an account and symbol.
func (c *Cache) GetOpenOrdersBySymbol(acctID int, symbolID int) []*common.Order {
	all := c.orderCache.GetOpenOrders(acctID)
	if all == nil {
		return nil
	}
	orders := make([]*common.Order, 0, len(all))
	for _, o := range all {
		if o.SymbolID == symbolID {
			orders = append(orders, o)
		}
	}
	return orders
}

// OpenOrderCount returns the number of open orders for an account.
func (c *Cache) OpenOrderCount(acctID int) int {
	return len(c.orderCache.GetOpenOrders(acctID))
}

// ============================================================================
// Order Write Methods
// ============================================================================

// InsertOrder inserts a new order into the cache.
func (c *Cache) InsertOrder(order *common.Order) {
	c.orderCache.InsertOrder(order)
}

// UpdateOrder updates an existing order and maintains the open-orders index.
func (c *Cache) UpdateOrder(order *common.Order) {
	c.orderCache.UpdateOrder(order)
}

// DeleteOrder removes an order from the cache.
func (c *Cache) DeleteOrder(clientOrderID int) {
	c.orderCache.DeleteOrder(clientOrderID)
}

// ============================================================================
// Balance Read Methods
// ============================================================================

// GetBalance returns the balance for a specific account and token.
func (c *Cache) GetBalance(acctID int, tokenID int) *common.Balance {
	acctBalances, ok := c.balances.Get(acctID)
	if !ok {
		return nil
	}
	bal, _ := acctBalances.Get(tokenID)
	return bal
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
func (c *Cache) GetAccountBalances(acctID int) map[int]*common.Balance {
	acctBalances, ok := c.balances.Get(acctID)
	if !ok {
		return nil
	}
	result := make(map[int]*common.Balance)
	acctBalances.ForEach(func(tokenID int, bal *common.Balance) bool {
		result[tokenID] = bal
		return true
	})
	return result
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
	acctBalances, _ := c.balances.GetOrCompute(acctID, func() *haxmap.Map[int, *common.Balance] {
		return haxmap.New[int, *common.Balance]()
	})
	acctBalances.Set(tokenID, &common.Balance{
		TokenID:   tokenID,
		Available: available,
		Locked:    locked,
		Total:     total,
	})
}

// SetAccountBalances replaces all balances for an account.
func (c *Cache) SetAccountBalances(acctID int, balances []common.Balance) {
	acctBalances := haxmap.New[int, *common.Balance](uintptr(len(balances)))
	for i := range balances {
		b := balances[i]
		acctBalances.Set(b.TokenID, &b)
	}
	c.balances.Set(acctID, acctBalances)
}

// ============================================================================
// Internal Helpers
// ============================================================================

func (c *Cache) ensureBook(symbolID int) *OrderBook {
	book, _ := c.books.GetOrCompute(symbolID, func() *OrderBook {
		return &OrderBook{
			state: common.BookStateWaitForSnapshot,
		}
	})
	return book
}
