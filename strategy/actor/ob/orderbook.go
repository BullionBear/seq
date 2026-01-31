package ob

import (
	"math"
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
	SymbolID       int
	PricePrecision int     // from catalog, e.g., 2 means 0.01 tick
	TickMultiplier float64 // = 10^PricePrecision, cached for conversion

	// State machine
	State   BookState
	DepthID int

	// Order book data (map-based BST)
	Bids *OrderedPriceMap // descending order (best bid = highest price first)
	Asks *OrderedPriceMap // ascending order (best ask = lowest price first)

	// Ring buffers for zero-copy storage
	BidLevelBuffer *PriceLevelRingBuffer
	AskLevelBuffer *PriceLevelRingBuffer

	// Update buffer (for WaitForSnapshot/Updating states)
	UpdateBuffer *DepthUpdateRingBuffer

	LastUpdated uint64
}

// NewSymbolOrderBook creates a new SymbolOrderBook with the given price precision.
func NewSymbolOrderBook(symbolID int, pricePrecision int) *SymbolOrderBook {
	return &SymbolOrderBook{
		SymbolID:       symbolID,
		PricePrecision: pricePrecision,
		TickMultiplier: math.Pow(10, float64(pricePrecision)),
		State:          StateWaitForSnapshot,
		DepthID:        0,
		Bids:           NewOrderedPriceMap(true),  // descending
		Asks:           NewOrderedPriceMap(false), // ascending
		BidLevelBuffer: NewPriceLevelRingBuffer(DefaultPriceLevelBufferSize),
		AskLevelBuffer: NewPriceLevelRingBuffer(DefaultPriceLevelBufferSize),
		UpdateBuffer:   NewDepthUpdateRingBuffer(DefaultDepthUpdateBufferSize),
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

// convertEventLevels converts event.PriceLevel slice to internal PriceLevel slice
func (sob *SymbolOrderBook) convertEventLevels(eventLevels []event.PriceLevel, buffer *PriceLevelRingBuffer) []PriceLevel {
	if len(eventLevels) == 0 {
		return nil
	}

	levels := buffer.Allocate(len(eventLevels))
	for i, el := range eventLevels {
		levels[i].PriceTick = sob.PriceToTick(el.Price)
		levels[i].Quantity = el.Quantity
	}
	return levels
}

// onDepthSnapshot handles a depth snapshot event
func (sob *SymbolOrderBook) onDepthSnapshot(snapshot event.DepthSnapshot) {
	log.Info().
		Int("symbolID", sob.SymbolID).
		Int("snapshotDepthID", snapshot.DepthID).
		Int("bids", len(snapshot.Bids)).
		Int("asks", len(snapshot.Asks)).
		Str("prevState", sob.State.String()).
		Msg("SymbolOrderBook: Processing snapshot")

	// Convert and load bids
	bidLevels := sob.convertEventLevels(snapshot.Bids, sob.BidLevelBuffer)
	sob.Bids.SetFromSlice(bidLevels)

	// Convert and load asks
	askLevels := sob.convertEventLevels(snapshot.Asks, sob.AskLevelBuffer)
	sob.Asks.SetFromSlice(askLevels)

	// Update state
	sob.DepthID = snapshot.DepthID
	sob.LastUpdated = snapshot.Timestamp

	// Transition to Updating state to process buffered updates
	sob.State = StateUpdating

	// Process any buffered updates
	sob.processBufferedUpdates()
}

// onDepthUpdate handles a depth update event
func (sob *SymbolOrderBook) onDepthUpdate(update event.DepthUpdate) {
	switch sob.State {
	case StateWaitForSnapshot:
		// Buffer the update for later processing
		sob.bufferUpdate(update)
		log.Debug().
			Int("symbolID", sob.SymbolID).
			Int("depthID", update.DepthID).
			Int("buffered", sob.UpdateBuffer.Count()).
			Msg("SymbolOrderBook: Buffered update (waiting for snapshot)")

	case StateUpdating:
		// Buffer and try to process
		sob.bufferUpdate(update)
		sob.processBufferedUpdates()

	case StateReady:
		// Apply directly if sequence is correct
		if update.PreviousDepthID == sob.DepthID {
			sob.applyUpdate(update)
		} else if update.PreviousDepthID > sob.DepthID {
			// We're behind - buffer and wait
			sob.bufferUpdate(update)
			log.Warn().
				Int("symbolID", sob.SymbolID).
				Int("currentDepthID", sob.DepthID).
				Int("updatePrevDepthID", update.PreviousDepthID).
				Msg("SymbolOrderBook: Gap detected, buffering update")
		}
		// If update.PreviousDepthID < sob.DepthID, it's stale - ignore it
	}
}

// bufferUpdate adds an update to the buffer
func (sob *SymbolOrderBook) bufferUpdate(update event.DepthUpdate) {
	// Convert price levels to internal format
	bidLevels := sob.convertEventLevels(update.Bids, sob.BidLevelBuffer)
	askLevels := sob.convertEventLevels(update.Asks, sob.AskLevelBuffer)

	bufferedUpdate := DepthUpdateBuffer{
		PreviousDepthID: update.PreviousDepthID,
		DepthID:         update.DepthID,
		Timestamp:       update.Timestamp,
		Bids:            bidLevels,
		Asks:            askLevels,
	}

	sob.UpdateBuffer.Push(bufferedUpdate)
}

// processBufferedUpdates processes buffered updates in order
func (sob *SymbolOrderBook) processBufferedUpdates() {
	processed := 0
	for !sob.UpdateBuffer.IsEmpty() {
		update := sob.UpdateBuffer.Peek()
		if update == nil {
			break
		}

		// Check if this update can be applied
		if update.PreviousDepthID <= sob.DepthID && update.DepthID > sob.DepthID {
			// This update is applicable
			sob.applyBufferedUpdate(update)
			sob.UpdateBuffer.Pop()
			processed++
		} else if update.DepthID <= sob.DepthID {
			// Stale update - discard
			sob.UpdateBuffer.Pop()
		} else {
			// Future update - can't apply yet, stop processing
			break
		}
	}

	// Transition to Ready if buffer is empty
	if sob.UpdateBuffer.IsEmpty() && sob.State == StateUpdating {
		sob.State = StateReady
		log.Info().
			Int("symbolID", sob.SymbolID).
			Int("depthID", sob.DepthID).
			Int("processedUpdates", processed).
			Msg("SymbolOrderBook: Transitioned to Ready state")
	}
}

// applyUpdate applies a depth update directly (for Ready state)
func (sob *SymbolOrderBook) applyUpdate(update event.DepthUpdate) {
	// Apply bid updates
	for _, level := range update.Bids {
		priceTick := sob.PriceToTick(level.Price)
		sob.Bids.Set(priceTick, level.Quantity)
	}

	// Apply ask updates
	for _, level := range update.Asks {
		priceTick := sob.PriceToTick(level.Price)
		sob.Asks.Set(priceTick, level.Quantity)
	}

	sob.DepthID = update.DepthID
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

	sob.DepthID = update.DepthID
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
	return sob.State == StateReady
}

// Reset resets the orderbook to WaitForSnapshot state
func (sob *SymbolOrderBook) Reset() {
	sob.State = StateWaitForSnapshot
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
		ActorBase: actor.NewActorBase("orderbook", []event.DataType{
			event.DataTypeDepthSnapshot,
			event.DataTypeDepthUpdate,
		}),
		books: make(map[int]*SymbolOrderBook),
	}
}

// RegisterSymbol registers a symbol with its price precision.
// This creates a new SymbolOrderBook in WaitForSnapshot state.
func (ob *OrderBook) RegisterSymbol(symbolID int, pricePrecision int) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if _, exists := ob.books[symbolID]; exists {
		log.Warn().Int("symbolID", symbolID).Msg("OrderBook: Symbol already registered")
		return
	}

	ob.books[symbolID] = NewSymbolOrderBook(symbolID, pricePrecision)
	log.Info().
		Int("symbolID", symbolID).
		Int("pricePrecision", pricePrecision).
		Msg("OrderBook: Registered symbol")
}

// Handle processes depth-related events to update the order book state.
func (ob *OrderBook) Handle(ev evbus.Event, bus *evbus.EventBus) {
	log.Debug().Msgf("Orderbook Actor: Handle called with event type: %d", ev.Ref.DataType)
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		ob.onDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		ob.onDepthUpdate(update)
	case event.DataTypeReqDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqDepthSnapshot(buf)
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

	book, exists := ob.books[snapshot.SymbolID]
	if !exists {
		log.Warn().
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
		log.Debug().
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
func (ob *OrderBook) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return nil, nil
	}

	// Get internal price levels
	bidLevels := book.GetTopBids(levels)
	askLevels := book.GetTopAsks(levels)

	// Convert to event.PriceLevel
	bids = make([]event.PriceLevel, len(bidLevels))
	for i, level := range bidLevels {
		bids[i] = event.PriceLevel{
			Price:    book.TickToPrice(level.PriceTick),
			Quantity: level.Quantity,
		}
	}

	asks = make([]event.PriceLevel, len(askLevels))
	for i, level := range askLevels {
		asks[i] = event.PriceLevel{
			Price:    book.TickToPrice(level.PriceTick),
			Quantity: level.Quantity,
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
func (ob *OrderBook) GetBookState(symbolID int) (BookState, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	book, exists := ob.books[symbolID]
	if !exists {
		return StateWaitForSnapshot, false
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
