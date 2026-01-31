package ob

import (
	"sort"
	"sync"

	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy/actor"
)

var log = logger.Get()

// Ensure OrderBook implements the Actor interface
var _ actor.Actor = (*OrderBook)(nil)

// Ensure OrderBook implements the OrderBookService interface
var _ actor.OrderBookService = (*OrderBook)(nil)

// SymbolOrderBook maintains the orderbook state for a single symbol.
type SymbolOrderBook struct {
	SymbolID    int
	DepthID     int
	Bids        []event.PriceLevel // sorted descending by price (best bid first)
	Asks        []event.PriceLevel // sorted ascending by price (best ask first)
	LastUpdated uint64
}

// OrderBook is an actor that maintains order book state from depth updates.
type OrderBook struct {
	actor.ActorBase
	mu    sync.RWMutex
	books map[int]*SymbolOrderBook // symbolID -> orderbook state
}

// NewOrderBook creates a new OrderBook actor.
func NewOrderBook() *OrderBook {
	return &OrderBook{
		ActorBase: actor.NewActorBase("orderbook", []event.DataType{
			event.DataTypeDepthSnapshot,
			event.DataTypeDepthUpdate,
		}),
		books: make(map[int]*SymbolOrderBook),
	}
}

// Handle processes depth-related events to update the order book state.
func (ob *OrderBook) Handle(ev evbus.Event, bus *evbus.EventBus) {
	log.Debug().Msgf("Orderbook Actor: Handle called with event type: %d", ev.Ref.DataType)
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		snapshot := bus.ReadDepthSnapshot(ev.Ref.Index)
		ob.onDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		update := bus.ReadDepthUpdate(ev.Ref.Index)
		ob.onDepthUpdate(update)
	case event.DataTypeReqDepthSnapshot:
		snapshot := bus.ReadReqDepthSnapshot(ev.Ref.Index)
		ob.onReqDepthSnapshot(snapshot)
	}
}

func (ob *OrderBook) onReqDepthSnapshot(snapshot event.ReqDepthSnapshot) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	log.Info().Msgf("Orderbook Actor: Req depth snapshot received: symbolID=%d", snapshot.SymbolID)
}

func (ob *OrderBook) onDepthSnapshot(snapshot event.DepthSnapshot) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	log.Info().Msgf("Orderbook Actor: Depth snapshot received: symbolID=%d, bids=%d, asks=%d", snapshot.SymbolID, len(snapshot.Bids), len(snapshot.Asks))

	// Create or replace the orderbook for this symbol
	book := &SymbolOrderBook{
		SymbolID:    snapshot.SymbolID,
		DepthID:     snapshot.DepthID,
		Bids:        make([]event.PriceLevel, len(snapshot.Bids)),
		Asks:        make([]event.PriceLevel, len(snapshot.Asks)),
		LastUpdated: snapshot.Timestamp,
	}

	// Copy bids and sort descending (best bid first)
	copy(book.Bids, snapshot.Bids)
	sort.Slice(book.Bids, func(i, j int) bool {
		return book.Bids[i].Price > book.Bids[j].Price
	})

	// Copy asks and sort ascending (best ask first)
	copy(book.Asks, snapshot.Asks)
	sort.Slice(book.Asks, func(i, j int) bool {
		return book.Asks[i].Price < book.Asks[j].Price
	})

	ob.books[snapshot.SymbolID] = book
}

func (ob *OrderBook) onDepthUpdate(update event.DepthUpdate) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	book, exists := ob.books[update.SymbolID]
	if !exists {
		// No snapshot received yet, skip update
		log.Debug().Msgf("Orderbook Actor: Skipping depth update (no snapshot yet): symbolID=%d", update.SymbolID)
		return
	}
	log.Info().Msgf("Orderbook Actor: Depth update: %d %d %d %d", update.SymbolID, update.DepthID, len(update.Bids), len(update.Asks))

	// Apply bid updates
	for _, level := range update.Bids {
		ob.applyLevelUpdate(&book.Bids, level, true)
	}

	// Apply ask updates
	for _, level := range update.Asks {
		ob.applyLevelUpdate(&book.Asks, level, false)
	}

	book.DepthID = update.DepthID
	book.LastUpdated = update.Timestamp
}

// applyLevelUpdate applies a single price level update to the orderbook side.
// For bids, descending=true; for asks, descending=false.
func (ob *OrderBook) applyLevelUpdate(levels *[]event.PriceLevel, update event.PriceLevel, descending bool) {
	// Find the position for this price
	idx := sort.Search(len(*levels), func(i int) bool {
		if descending {
			return (*levels)[i].Price <= update.Price
		}
		return (*levels)[i].Price >= update.Price
	})

	if update.Quantity == 0 {
		// Remove the level if it exists
		if idx < len(*levels) && (*levels)[idx].Price == update.Price {
			*levels = append((*levels)[:idx], (*levels)[idx+1:]...)
		}
	} else {
		// Update or insert the level
		if idx < len(*levels) && (*levels)[idx].Price == update.Price {
			// Update existing level
			(*levels)[idx].Quantity = update.Quantity
		} else {
			// Insert new level
			*levels = append(*levels, event.PriceLevel{})
			copy((*levels)[idx+1:], (*levels)[idx:])
			(*levels)[idx] = update
		}
	}
}

// GetBestBid returns the best bid price and quantity for the given symbol.
func (ob *OrderBook) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || len(book.Bids) == 0 {
		return 0, 0, false
	}
	return book.Bids[0].Price, book.Bids[0].Quantity, true
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (ob *OrderBook) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || len(book.Asks) == 0 {
		return 0, 0, false
	}
	return book.Asks[0].Price, book.Asks[0].Quantity, true
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (ob *OrderBook) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return nil, nil
	}

	// Copy bids
	bidCount := levels
	if bidCount > len(book.Bids) {
		bidCount = len(book.Bids)
	}
	bids = make([]event.PriceLevel, bidCount)
	copy(bids, book.Bids[:bidCount])

	// Copy asks
	askCount := levels
	if askCount > len(book.Asks) {
		askCount = len(book.Asks)
	}
	asks = make([]event.PriceLevel, askCount)
	copy(asks, book.Asks[:askCount])

	return bids, asks
}

// GetMidPrice returns the mid price for the given symbol.
func (ob *OrderBook) GetMidPrice(symbolID int) (price float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || len(book.Bids) == 0 || len(book.Asks) == 0 {
		return 0, false
	}
	return (book.Bids[0].Price + book.Asks[0].Price) / 2, true
}

// GetSpread returns the bid-ask spread for the given symbol.
func (ob *OrderBook) GetSpread(symbolID int) (spread float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || len(book.Bids) == 0 || len(book.Asks) == 0 {
		return 0, false
	}
	return book.Asks[0].Price - book.Bids[0].Price, true
}
