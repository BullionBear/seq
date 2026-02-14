package data

import (
	"context"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	dactor "github.com/BullionBear/seq/data/actor"
	"github.com/BullionBear/seq/data/ob"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/msgbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// DataSubscription holds parsed subscription config for a single symbol
type DataSubscription struct {
	SymbolID int
	Endpoint string                // regional endpoint override
	Depth    *adapter.DepthOptions // nil if no depth subscription
	Trade    *adapter.TradeOptions // nil if no trade subscription
}

// Engine manages market data processing including orderbook management.
// It owns the OrderBook and SnapshotCoordinator actors, and registers them
// with the EventBus. The engine itself is NOT an actor.
type Engine struct {
	catalog              *catalog.Catalog
	msgBus               *msgbus.MsgBus
	orderBook            *ob.OrderBook
	snapshotCoordinator  *dactor.Coordinator
	router               *adapter.DataRouter

	// Data subscriptions from config
	dataSubs []DataSubscription
}

// NewEngine creates a new data engine.
func NewEngine(cat *catalog.Catalog, msgBus *msgbus.MsgBus) *Engine {
	orderBook := ob.NewOrderBook()
	router := adapter.NewDataRouter(cat, msgBus)
	return &Engine{
		catalog:             cat,
		msgBus:              msgBus,
		orderBook:           orderBook,
		snapshotCoordinator: dactor.NewCoordinator(orderBook, router),
		router:              router,
	}
}

// Init registers the engine's actors with the MsgBus.
func (e *Engine) Init() {
	actor.Register(e.msgBus, e.orderBook)
	actor.Register(e.msgBus, e.snapshotCoordinator)
	log().Info().Msg("DataEngine initialized")
}

// ============================================================================
// Config-based Subscription Methods
// ============================================================================

// SetDataConfig parses config and stores subscription requirements.
// Call this before Connect() to configure what data to subscribe to.
func (e *Engine) SetDataConfig(config []strategy.ConfigDataSub) error {
	e.dataSubs = make([]DataSubscription, 0, len(config))

	for _, cfg := range config {
		symbol, err := e.catalog.GetSymbolByUniversalTicker(cfg.Symbol)
		if err != nil {
			log().Error().Err(err).Str("symbol", cfg.Symbol).Msg("DataEngine: Failed to resolve symbol from config")
			return err
		}

		sub := DataSubscription{
			SymbolID: symbol.ID,
			Endpoint: cfg.Endpoint,
		}

		// Parse depth options
		if cfg.Depth != nil {
			sub.Depth = &adapter.DepthOptions{
				Type:     cfg.Depth.Type,
				PushRate: cfg.Depth.PushRate,
				Levels:   cfg.Depth.Levels,
			}
		}

		// Parse trade options
		if cfg.Trade != nil {
			sub.Trade = &adapter.TradeOptions{
				Type: cfg.Trade.Type,
			}
		}

		e.dataSubs = append(e.dataSubs, sub)
		log().Debug().
			Int("symbolID", symbol.ID).
			Str("ticker", cfg.Symbol).
			Bool("hasDepth", sub.Depth != nil).
			Bool("hasTrade", sub.Trade != nil).
			Msg("DataEngine: Configured subscription")
	}

	return nil
}

// ============================================================================
// Legacy Subscription Methods (for backward compatibility)
// ============================================================================

// SubscribeDepthUpdate subscribes to depth updates for a symbol.
// Deprecated: Use SetDataConfig() instead.
func (e *Engine) SubscribeDepthUpdate(symbolID int) {
	// Get symbol from catalog to get price precision
	symbol, err := e.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to get symbol from catalog")
		return
	}

	// Register symbol with orderbook
	e.orderBook.RegisterSymbol(symbolID, symbol.PricePrecision)

	// Subscribe to depth updates via router with default options
	if err := e.router.SubscribeDepthUpdate(symbolID, nil); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to depth update")
	}
}

// SubscribeTick subscribes to tick updates for a symbol.
// Deprecated: Use SetDataConfig() instead.
func (e *Engine) SubscribeTick(symbolID int) {
	if err := e.router.SubscribeTrade(symbolID, nil); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to tick")
	}
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect subscribes to all configured data streams and connects to data sources.
func (e *Engine) Connect(ctx context.Context) {
	// Subscribe to all configured data streams
	for _, sub := range e.dataSubs {
		// Get symbol for orderbook registration
		symbol, err := e.catalog.GetSymbol(sub.SymbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to get symbol")
			continue
		}

		// Subscribe to depth if configured
		if sub.Depth != nil {
			// Register symbol with orderbook
			e.orderBook.RegisterSymbol(sub.SymbolID, symbol.PricePrecision)

			// Subscribe via router with options
			if err := e.router.SubscribeDepthUpdate(sub.SymbolID, sub.Depth); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to depth")
			}
		}

		// Subscribe to trade if configured
		if sub.Trade != nil {
			if err := e.router.SubscribeTrade(sub.SymbolID, sub.Trade); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to trade")
			}
		}
	}

	// Connect router (which connects exchange clients)
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
