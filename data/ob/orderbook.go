package ob

import (
	"math"
	"sync"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

const (
	// DefaultPriceLevelBufferSize is the default capacity for price level arenas
	DefaultPriceLevelBufferSize = 1000

	// DefaultDepthUpdateBufferSize is the default capacity for depth update ring buffers
	DefaultDepthUpdateBufferSize = 100
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure OrderBook implements the Actor interface
var _ actor.Actor = (*OrderBook)(nil)

// Ensure OrderBook implements the OrderBookService interface
var _ OrderBookService = (*OrderBook)(nil)

// SymbolOrderBook maintains the orderbook state for a single symbol.
type SymbolOrderBook struct {
	SymbolID        int
	PricePrecision  int     // from catalog, e.g., 2 means 0.01 tick
	SizePrecision   int     // from catalog, e.g., 5 means 0.00001 quantity tick
	TickMultiplier  float64 // = 10^PricePrecision, cached for conversion
	SizeMultiplier  float64 // = 10^SizePrecision, cached for quantity tick conversion

	// State machine
	State   common.BookState
	DepthID int

	// Order book data (map-based BST)
	Bids *OrderedPriceMap // descending order (best bid = highest price first)
	Asks *OrderedPriceMap // ascending order (best ask = lowest price first)

	// Arenas for zero-copy storage of price levels
	BidLevelArena *mem.SliceArena[PriceLevel]
	AskLevelArena *mem.SliceArena[PriceLevel]

	// Update buffer (for WaitForSnapshot/Updating states) - SPSC ring buffer
	UpdateBuffer *mem.SPSCRingBuffer[DepthUpdateBuffer]

	LastUpdated uint64
}

// NewSymbolOrderBook creates a new SymbolOrderBook with the given price and size precision.
func NewSymbolOrderBook(symbolID int, pricePrecision, sizePrecision int) *SymbolOrderBook {
	return &SymbolOrderBook{
		SymbolID:       symbolID,
		PricePrecision: pricePrecision,
		SizePrecision:  sizePrecision,
		TickMultiplier: math.Pow(10, float64(pricePrecision)),
		SizeMultiplier: math.Pow(10, float64(sizePrecision)),
		State:          common.BookStateWaitForSnapshot,
		DepthID:        0,
		Bids:           NewOrderedPriceMap(true),  // descending
		Asks:           NewOrderedPriceMap(false), // ascending
		BidLevelArena:  mem.NewSliceArena[PriceLevel](DefaultPriceLevelBufferSize),
		AskLevelArena:  mem.NewSliceArena[PriceLevel](DefaultPriceLevelBufferSize),
		UpdateBuffer:   mem.NewSPSCRingBuffer[DepthUpdateBuffer](DefaultDepthUpdateBufferSize),
		LastUpdated:    0,
	}
}

// PriceToTick converts float price to integer tick
func (sob *SymbolOrderBook) PriceToTick(price float64) int64 {
	return int64(math.Round(price * sob.TickMultiplier))
}

// TickToPrice converts integer tick back to float price
func (sob *SymbolOrderBook) TickToPrice(tick int64) float64 {
	return float64(tick) / sob.TickMultiplier
}

// convertEventLevels converts event.PriceLevel slice to internal PriceLevel slice.
// Uses PriceTick from event when available (populated by exchange adapters).
func (sob *SymbolOrderBook) convertEventLevels(eventLevels []common.PriceLevel, arena *mem.SliceArena[PriceLevel]) []PriceLevel {
	if len(eventLevels) == 0 {
		return nil
	}

	levels := arena.Allocate(len(eventLevels))
	for i, el := range eventLevels {
		// Use PriceTick from event (populated by adapters from catalog precision)
		levels[i].PriceTick = int64(el.PriceTick)
		levels[i].Quantity = el.Quantity
	}
	return levels
}

// onDepthSnapshot handles a depth snapshot event
func (sob *SymbolOrderBook) onDepthSnapshot(snapshot event.DepthSnapshot) {
	log().Info().
		Int("symbolID", sob.SymbolID).
		Int("snapshotDepthID", snapshot.DepthID).
		Int("bids", len(snapshot.Bids)).
		Int("asks", len(snapshot.Asks)).
		Str("prevState", sob.State.String()).
		Msg("SymbolOrderBook: Processing snapshot")

	// Convert and load bids
	bidLevels := sob.convertEventLevels(snapshot.Bids, sob.BidLevelArena)
	sob.Bids.SetFromSlice(bidLevels)

	// Convert and load asks
	askLevels := sob.convertEventLevels(snapshot.Asks, sob.AskLevelArena)
	sob.Asks.SetFromSlice(askLevels)

	// Update state
	sob.DepthID = snapshot.DepthID
	sob.LastUpdated = snapshot.Timestamp

	// Transition to Updating state to process buffered updates
	sob.State = common.BookStateUpdating

	// Process any buffered updates
	sob.processBufferedUpdates()
}

// onDepthUpdate handles a depth update event
func (sob *SymbolOrderBook) onDepthUpdate(update event.DepthUpdate) {
	switch sob.State {
	case common.BookStateWaitForSnapshot:
		// Buffer the update for later processing
		sob.bufferUpdate(update)
		log().Debug().
			Int("symbolID", sob.SymbolID).
			Int("depthID", update.DepthID).
			Uint64("buffered", sob.UpdateBuffer.Count()).
			Msg("SymbolOrderBook: Buffered update (waiting for snapshot)")

	case common.BookStateUpdating:
		// Buffer and try to process
		sob.bufferUpdate(update)
		sob.processBufferedUpdates()

	case common.BookStateReady:
		// For Binance's aggregated diff stream (@depth@100ms):
		// - PreviousDepthID = U - 1 (one before first update ID)
		// - CurrentDepthID = u (final update ID in event)
		// - sob.DepthID stores the last final update ID (prev_u)
		// Accept updates where: PreviousDepthID <= sob.DepthID < CurrentDepthID
		// This means: next update's U-1 should be <= our last u
		if update.PreviousDepthID <= sob.DepthID && update.CurrentDepthID > sob.DepthID {
			// This update covers our current position - apply it
			sob.applyUpdate(update)
			log().Debug().
				Int("symbolID", sob.SymbolID).
				Int("prevDepthID", update.PreviousDepthID).
				Int("bookDepthID", sob.DepthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Msg("SymbolOrderBook: Applied update")
		} else if update.CurrentDepthID <= sob.DepthID {
			// Stale update - ignore it
			log().Debug().
				Int("symbolID", sob.SymbolID).
				Int("bookDepthID", sob.DepthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Msg("SymbolOrderBook: Ignoring stale update")
		} else {
			// Future update (PreviousDepthID > sob.DepthID) - we missed some updates
			// This indicates real data loss - reset and wait for new snapshot
			gap := update.PreviousDepthID - sob.DepthID
			log().Error().
				Int("symbolID", sob.SymbolID).
				Int("bookDepthID", sob.DepthID).
				Int("updatePrevDepthID", update.PreviousDepthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Int("missedDepthIDs", gap).
				Msg("SymbolOrderBook: Data loss detected, resetting to WaitForSnapshot")
			// Reset the orderbook - strategy will request new snapshot
			sob.Reset()
		}
	}
}

// bufferUpdate adds an update to the buffer
func (sob *SymbolOrderBook) bufferUpdate(update event.DepthUpdate) {
	// Convert price levels to internal format
	bidLevels := sob.convertEventLevels(update.Bids, sob.BidLevelArena)
	askLevels := sob.convertEventLevels(update.Asks, sob.AskLevelArena)

	// For Binance:
	// - PreviousDepthID = U - 1
	// - DepthID = U (first update ID)
	// - CurrentDepthID = u (final update ID)
	bufferedUpdate := DepthUpdateBuffer{
		PreviousDepthID: update.PreviousDepthID,
		FirstDepthID:    update.DepthID,
		FinalDepthID:    update.CurrentDepthID,
		Timestamp:       update.Timestamp,
		Bids:            bidLevels,
		Asks:            askLevels,
	}

	// Write to SPSC ring buffer (will return false if full, but we log anyway)
	if !sob.UpdateBuffer.Write(bufferedUpdate) {
		log().Warn().
			Int("symbolID", sob.SymbolID).
			Msg("SymbolOrderBook: Update buffer full, dropping update")
	}
}

// processBufferedUpdates processes buffered updates in order
func (sob *SymbolOrderBook) processBufferedUpdates() {
	processed := 0
	for !sob.UpdateBuffer.IsEmpty() {
		update, ok := sob.UpdateBuffer.Peek()
		if !ok {
			break
		}

		// Check if this update can be applied
		// For Binance: PreviousDepthID = U-1, FinalDepthID = u
		// An update is applicable if: PreviousDepthID <= currentDepthID < FinalDepthID
		// This means the update's range covers our current position
		if update.PreviousDepthID <= sob.DepthID && update.FinalDepthID > sob.DepthID {
			// This update is applicable
			sob.applyBufferedUpdate(&update)
			sob.UpdateBuffer.Read() // consume the item
			processed++
		} else if update.FinalDepthID <= sob.DepthID {
			// Stale update - discard
			sob.UpdateBuffer.Read() // consume the item
		} else {
			// Future update - can't apply yet, stop processing
			break
		}
	}

	// Transition to Ready if buffer is empty
	if sob.UpdateBuffer.IsEmpty() && sob.State == common.BookStateUpdating {
		sob.State = common.BookStateReady
		log().Info().
			Int("symbolID", sob.SymbolID).
			Int("depthID", sob.DepthID).
			Int("processedUpdates", processed).
			Msg("SymbolOrderBook: Transitioned to Ready state")
	}
}

// applyUpdate applies a depth update directly (for Ready state)
func (sob *SymbolOrderBook) applyUpdate(update event.DepthUpdate) {
	// Apply bid updates (use PriceTick from event, populated by adapters)
	for _, level := range update.Bids {
		sob.Bids.Set(int64(level.PriceTick), level.Quantity)
	}

	// Apply ask updates
	for _, level := range update.Asks {
		sob.Asks.Set(int64(level.PriceTick), level.Quantity)
	}

	// Store the FINAL update ID (u) so next update's PreviousDepthID (U-1) can match
	// For Binance: CurrentDepthID = u (final update ID in event)
	sob.DepthID = update.CurrentDepthID
	sob.LastUpdated = update.Timestamp
}

// applyBufferedUpdate applies a buffered update
func (sob *SymbolOrderBook) applyBufferedUpdate(update *DepthUpdateBuffer) {
	// Apply bid updates
	for _, level := range update.Bids {
		sob.Bids.Set(level.PriceTick, level.Quantity)
	}

	// Apply ask updates
	for _, level := range update.Asks {
		sob.Asks.Set(level.PriceTick, level.Quantity)
	}

	// Store the FINAL update ID (u) so next update's PreviousDepthID (U-1) can match
	sob.DepthID = update.FinalDepthID
	sob.LastUpdated = update.Timestamp
}

// GetBestBid returns the best bid price tick and quantity
func (sob *SymbolOrderBook) GetBestBid() (priceTick int64, qty float64, ok bool) {
	return sob.Bids.GetBest()
}

// GetBestAsk returns the best ask price tick and quantity
func (sob *SymbolOrderBook) GetBestAsk() (priceTick int64, qty float64, ok bool) {
	return sob.Asks.GetBest()
}

// GetTopBids returns the top N bid levels
func (sob *SymbolOrderBook) GetTopBids(n int) []PriceLevel {
	return sob.Bids.GetTopN(n)
}

// GetTopAsks returns the top N ask levels
func (sob *SymbolOrderBook) GetTopAsks(n int) []PriceLevel {
	return sob.Asks.GetTopN(n)
}

// IsReady returns true if the orderbook is in Ready state
func (sob *SymbolOrderBook) IsReady() bool {
	return sob.State == common.BookStateReady
}

// Reset resets the orderbook to WaitForSnapshot state
func (sob *SymbolOrderBook) Reset() {
	sob.State = common.BookStateWaitForSnapshot
	sob.DepthID = 0
	sob.Bids.Clear()
	sob.Asks.Clear()
	sob.UpdateBuffer.Reset()
	sob.LastUpdated = 0
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
		ActorBase: actor.NewActorBase("orderbook", []event.Topic{
			event.TopicEventDepthSnapshot,
			event.TopicEventDepthUpdate,
		}),
		books: make(map[int]*SymbolOrderBook),
	}
}

// RegisterSymbol registers a symbol with its price and size precision.
// This creates a new SymbolOrderBook in WaitForSnapshot state.
func (ob *OrderBook) RegisterSymbol(symbolID int, pricePrecision, sizePrecision int) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if _, exists := ob.books[symbolID]; exists {
		log().Warn().Int("symbolID", symbolID).Msg("OrderBook: Symbol already registered")
		return
	}

	ob.books[symbolID] = NewSymbolOrderBook(symbolID, pricePrecision, sizePrecision)
	log().Info().
		Int("symbolID", symbolID).
		Int("pricePrecision", pricePrecision).
		Int("sizePrecision", sizePrecision).
		Msg("OrderBook: Registered symbol")
}

// Handle processes depth-related events to update the order book state.
func (ob *OrderBook) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	log().Debug().Msgf("Orderbook Actor: Handle called with topic: %d", ev.Ref.Topic)
	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewDepthSnapshotFromBytes(buf)
		ob.onDepthSnapshot(snapshot)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		ob.onDepthUpdate(update)
	}
}

// OnInit is called once when the actor is initialized.
func (ob *OrderBook) OnInit() {
	log().Info().Msg("OrderBook: initialized")
}

// OnStart is called once when the actor is started.
func (ob *OrderBook) OnStart() {
	log().Info().Msg("OrderBook: started")
}

// OnStop is called once when the actor is stopped.
// It resets all registered symbol orderbooks to a clean state.
func (ob *OrderBook) OnStop() {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	for _, book := range ob.books {
		book.Reset()
	}
	log().Info().Int("symbols", len(ob.books)).Msg("OrderBook: stopped, all books reset")
}

func (ob *OrderBook) onDepthSnapshot(snapshot event.DepthSnapshot) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	book, exists := ob.books[snapshot.SymbolID]
	if !exists {
		log().Warn().
			Int("symbolID", snapshot.SymbolID).
			Msg("OrderBook: Received snapshot for unregistered symbol")
		return
	}

	book.onDepthSnapshot(snapshot)
}

func (ob *OrderBook) onDepthUpdate(update event.DepthUpdate) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	book, exists := ob.books[update.SymbolID]
	if !exists {
		log().Debug().
			Int("symbolID", update.SymbolID).
			Msg("OrderBook: Received update for unregistered symbol")
		return
	}

	book.onDepthUpdate(update)
}

// GetBestBid returns the best bid price and quantity for the given symbol.
func (ob *OrderBook) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || !book.IsReady() {
		return 0, 0, false
	}

	priceTick, qty, ok := book.GetBestBid()
	if !ok {
		return 0, 0, false
	}
	return book.TickToPrice(priceTick), qty, true
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (ob *OrderBook) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || !book.IsReady() {
		return 0, 0, false
	}

	priceTick, qty, ok := book.GetBestAsk()
	if !ok {
		return 0, 0, false
	}
	return book.TickToPrice(priceTick), qty, true
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
// Returns event.PriceLevel for compatibility with the OrderBookService interface.
func (ob *OrderBook) GetDepth(symbolID int, levels int) (bids, asks []common.PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return nil, nil
	}

	// Get internal price levels
	bidLevels := book.GetTopBids(levels)
	askLevels := book.GetTopAsks(levels)

	// Convert to event.PriceLevel with PriceTick and QuantityTick
	bids = make([]common.PriceLevel, len(bidLevels))
	for i, level := range bidLevels {
		bids[i] = common.PriceLevel{
			Price:        book.TickToPrice(level.PriceTick),
			Quantity:      level.Quantity,
			PriceTick:     int(level.PriceTick),
			QuantityTick: common.QuantityToTick(level.Quantity, book.SizePrecision),
		}
	}

	asks = make([]common.PriceLevel, len(askLevels))
	for i, level := range askLevels {
		asks[i] = common.PriceLevel{
			Price:        book.TickToPrice(level.PriceTick),
			Quantity:      level.Quantity,
			PriceTick:     int(level.PriceTick),
			QuantityTick: common.QuantityToTick(level.Quantity, book.SizePrecision),
		}
	}

	return bids, asks
}

// GetMidPrice returns the mid price for the given symbol.
func (ob *OrderBook) GetMidPrice(symbolID int) (price float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || !book.IsReady() {
		return 0, false
	}

	bidTick, _, bidOk := book.GetBestBid()
	askTick, _, askOk := book.GetBestAsk()
	if !bidOk || !askOk {
		return 0, false
	}

	midTick := (bidTick + askTick) / 2
	return book.TickToPrice(midTick), true
}

// GetSpread returns the bid-ask spread for the given symbol.
func (ob *OrderBook) GetSpread(symbolID int) (spread float64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists || !book.IsReady() {
		return 0, false
	}

	bidTick, _, bidOk := book.GetBestBid()
	askTick, _, askOk := book.GetBestAsk()
	if !bidOk || !askOk {
		return 0, false
	}

	spreadTicks := askTick - bidTick
	return book.TickToPrice(spreadTicks), true
}

// GetBookState returns the state of the orderbook for a symbol.
func (ob *OrderBook) GetBookState(symbolID int) (common.BookState, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return common.BookStateWaitForSnapshot, false
	}
	return book.State, true
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (ob *OrderBook) IsSymbolReady(symbolID int) bool {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return false
	}
	return book.IsReady()
}
