package data

import (
	"context"
	"sync"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data/ob"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure Engine implements the Actor interface
var _ actor.Actor = (*Engine)(nil)

// Engine manages market data processing including orderbook management.
// It owns the OrderBook actor and DataRouter, and handles the snapshot
// workflow automatically so strategies don't need to manage it.
type Engine struct {
	actor.ActorBase
	catalog   *catalog.Catalog
	eventBus  *evbus.EventBus
	orderBook *ob.OrderBook
	router    *adapter.DataRouter

	// Snapshot request tracking
	mu               sync.RWMutex
	pendingSnapshots map[int]bool // symbolID -> snapshot requested but not received
}

// NewEngine creates a new data engine.
func NewEngine(cat *catalog.Catalog, eventBus *evbus.EventBus) *Engine {
	return &Engine{
		ActorBase: actor.NewActorBase("data-engine", []event.Topic{
			event.TopicEventDepthUpdate,
			event.TopicEventReqDepthSnapshot,
		}),
		catalog:          cat,
		eventBus:         eventBus,
		orderBook:        ob.NewOrderBook(),
		router:           adapter.NewDataRouter(cat, eventBus),
		pendingSnapshots: make(map[int]bool),
	}
}

// Init initializes the data engine.
// It registers the OrderBook as an actor with the EventBus.
func (e *Engine) Init() {
	// Register the orderbook actor with the EventBus
	actor.Register(e.eventBus, e.orderBook)
	log().Info().Msg("DataEngine initialized")
}

// Handle processes depth-related events.
// It automatically requests snapshots when orderbook is in WaitForSnapshot state.
func (e *Engine) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		e.onDepthUpdate(update)
	case event.TopicEventReqDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqDepthSnapshot(buf)
		e.onReqDepthSnapshot(snapshot)
	}
}

// onDepthUpdate handles depth update events.
// It checks if the orderbook needs a snapshot and auto-requests one.
func (e *Engine) onDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID

	// Check if orderbook is waiting for snapshot
	state, exists := e.orderBook.GetBookState(symbolID)
	if !exists {
		return
	}

	// If waiting for snapshot and not already requested, request one
	if state == ob.StateWaitForSnapshot {
		e.mu.Lock()
		if !e.pendingSnapshots[symbolID] {
			e.pendingSnapshots[symbolID] = true
			e.mu.Unlock()

			log().Debug().Int("symbolID", symbolID).Msg("DataEngine: Auto-requesting depth snapshot")
			if err := e.router.ReqDepthSnapshot(symbolID); err != nil {
				log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to request depth snapshot")
				e.mu.Lock()
				e.pendingSnapshots[symbolID] = false
				e.mu.Unlock()
			}
		} else {
			e.mu.Unlock()
		}
	}
}

// onReqDepthSnapshot handles the response to a depth snapshot request.
// It clears the pending snapshot flag for the symbol.
func (e *Engine) onReqDepthSnapshot(snapshot event.ReqDepthSnapshot) {
	symbolID := snapshot.SymbolID
	e.mu.Lock()
	delete(e.pendingSnapshots, symbolID)
	e.mu.Unlock()
	log().Debug().Int("symbolID", symbolID).Msg("DataEngine: Depth snapshot received, cleared pending flag")
}

// ============================================================================
// Subscription Methods
// ============================================================================

// SubscribeDepthUpdate subscribes to depth updates for a symbol.
func (e *Engine) SubscribeDepthUpdate(symbolID int) {
	// Get symbol from catalog to get price precision
	symbol, err := e.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to get symbol from catalog")
		return
	}

	// Register symbol with orderbook
	e.orderBook.RegisterSymbol(symbolID, symbol.PricePrecision)

	// Subscribe to depth updates via router
	if err := e.router.SubscribeDepthUpdate(symbolID); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to depth update")
	}
}

// SubscribeTick subscribes to tick updates for a symbol.
func (e *Engine) SubscribeTick(symbolID int) {
	if err := e.router.SubscribeTrade(symbolID); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to tick")
	}
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect connects to all data sources.
func (e *Engine) Connect(ctx context.Context) {
	if err := e.router.Connect(ctx); err != nil {
		log().Error().Err(err).Msg("DataEngine: Failed to connect to data router")
	}
}

// Disconnect disconnects from all data sources.
func (e *Engine) Disconnect() {
	e.router.Disconnect()
}

// ============================================================================
// OrderBook Query Methods (implements OrderBookService interface)
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (e *Engine) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	return e.orderBook.GetBestBid(symbolID)
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (e *Engine) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	return e.orderBook.GetBestAsk(symbolID)
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (e *Engine) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	return e.orderBook.GetDepth(symbolID, levels)
}

// GetMidPrice returns the mid price for the given symbol.
func (e *Engine) GetMidPrice(symbolID int) (price float64, ok bool) {
	return e.orderBook.GetMidPrice(symbolID)
}

// GetSpread returns the bid-ask spread for the given symbol.
func (e *Engine) GetSpread(symbolID int) (spread float64, ok bool) {
	return e.orderBook.GetSpread(symbolID)
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (e *Engine) IsSymbolReady(symbolID int) bool {
	return e.orderBook.IsSymbolReady(symbolID)
}

// GetBookState returns the state of the orderbook for a symbol.
func (e *Engine) GetBookState(symbolID int) (ob.BookState, bool) {
	return e.orderBook.GetBookState(symbolID)
}

// Catalog returns the catalog.
func (e *Engine) Catalog() *catalog.Catalog {
	return e.catalog
}
