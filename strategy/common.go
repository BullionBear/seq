package strategy

import (
	"context"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy/actor"
	"github.com/BullionBear/seq/strategy/actor/ems"
	"github.com/BullionBear/seq/strategy/actor/ob"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// StrategyCommon provides the common infrastructure for all strategies.
// It owns and manages internal actors (OrderBook, EMS) and provides
// a clean facade API for strategies to interact with market data and orders.
type StrategyCommon struct {
	eventBus   *evbus.EventBus
	catalog    *catalog.Catalog
	dataRouter *adapter.DataRouter

	// Internal actors (facade owns these)
	orderBook *ob.OrderBook
	ems       *ems.EMS
}

// NewStrategyCommon creates a new StrategyCommon with internal actors.
// It registers the actors with the EventBus for event processing.
func NewStrategyCommon(catalog *catalog.Catalog, eventBus *evbus.EventBus) *StrategyCommon {
	// Create internal actors
	orderBook := ob.NewOrderBook()
	emsActor := ems.NewEMS()
	emsActor.SetEventBus(eventBus)

	// Register actors with EventBus
	actor.Register(eventBus, orderBook)
	actor.Register(eventBus, emsActor)

	return &StrategyCommon{
		eventBus:   eventBus,
		catalog:    catalog,
		dataRouter: adapter.NewDataRouter(catalog, eventBus),
		orderBook:  orderBook,
		ems:        emsActor,
	}
}

// Getters
func (s *StrategyCommon) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// ============================================================================
// OrderBook Service Methods (delegated to OrderBook agent)
// ============================================================================

// GetBestBid returns the best bid price and quantity for the given symbol.
func (s *StrategyCommon) GetBestBid(symbolID int) (price, qty float64, ok bool) {
	return s.orderBook.GetBestBid(symbolID)
}

// GetBestAsk returns the best ask price and quantity for the given symbol.
func (s *StrategyCommon) GetBestAsk(symbolID int) (price, qty float64, ok bool) {
	return s.orderBook.GetBestAsk(symbolID)
}

// GetDepth returns the top N levels of bids and asks for the given symbol.
func (s *StrategyCommon) GetDepth(symbolID int, levels int) (bids, asks []event.PriceLevel) {
	return s.orderBook.GetDepth(symbolID, levels)
}

// GetMidPrice returns the mid price for the given symbol.
func (s *StrategyCommon) GetMidPrice(symbolID int) (price float64, ok bool) {
	return s.orderBook.GetMidPrice(symbolID)
}

// GetSpread returns the bid-ask spread for the given symbol.
func (s *StrategyCommon) GetSpread(symbolID int) (spread float64, ok bool) {
	return s.orderBook.GetSpread(symbolID)
}

// ============================================================================
// Order Management Methods (delegated to EMS agent)
// ============================================================================

// SubmitLimitOrder submits a new limit order and returns the client order ID.
func (s *StrategyCommon) SubmitLimitOrder(acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) int {
	req := actor.LimitOrderRequest{
		AccountID:   acctID,
		SymbolID:    symbolID,
		Side:        side,
		TimeInForce: timeInForce,
		Quantity:    quantity,
		Price:       price,
	}
	clientOrderID, err := s.ems.SubmitLimitOrder(req)
	if err != nil {
		log().Error().Err(err).Msg("Failed to submit limit order")
		return -1
	}
	return clientOrderID
}

// SubmitMarketOrder submits a new market order and returns the client order ID.
func (s *StrategyCommon) SubmitMarketOrder(acctID int, symbolID int, side common.Side, quantity float64) int {
	req := actor.MarketOrderRequest{
		AccountID: acctID,
		SymbolID:  symbolID,
		Side:      side,
		Quantity:  quantity,
	}
	clientOrderID, err := s.ems.SubmitMarketOrder(req)
	if err != nil {
		log().Error().Err(err).Msg("Failed to submit market order")
		return -1
	}
	return clientOrderID
}

// CancelOrder cancels an existing order.
func (s *StrategyCommon) CancelOrder(clientOrderID int) error {
	return s.ems.CancelOrder(clientOrderID)
}

// GetOrder retrieves an order by its client order ID.
func (s *StrategyCommon) GetOrder(clientOrderID int) (common.Order, error) {
	return s.ems.GetOrder(clientOrderID)
}

// GetOpenOrders returns all currently open orders.
func (s *StrategyCommon) GetOpenOrders() []common.Order {
	return s.ems.GetOpenOrders()
}

// GetOrdersBySymbol returns all orders for a specific symbol.
func (s *StrategyCommon) GetOrdersBySymbol(symbolID int) []common.Order {
	return s.ems.GetOrdersBySymbol(symbolID)
}

// Private subscription methods
func (s *StrategyCommon) SubscribeOrderUpdate(acctID int) {
}

func (s *StrategyCommon) SubscribeBalanceUpdate(acctID int) {
}

func (s *StrategyCommon) SubscribeOrderFill(acctID int) {
}

// IsSymbolReady returns true if the orderbook for a symbol is ready.
func (s *StrategyCommon) IsSymbolReady(symbolID int) bool {
	return s.orderBook.IsSymbolReady(symbolID)
}

// GetBookState returns the state of the orderbook for a symbol.
func (s *StrategyCommon) GetBookState(symbolID int) (ob.BookState, bool) {
	return s.orderBook.GetBookState(symbolID)
}

// Public subscription methods
func (s *StrategyCommon) SubscribeDepthUpdate(symbolID int) {
	// Get symbol from catalog to get price precision
	symbol, err := s.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Msgf("Failed to get symbol from catalog: %d", symbolID)
		return
	}

	// Register symbol with orderbook (with price precision from catalog)
	s.orderBook.RegisterSymbol(symbolID, symbol.PricePrecision)

	// Subscribe to depth updates via data router
	err = s.dataRouter.SubscribeDepthUpdate(symbolID)
	if err != nil {
		log().Error().Err(err).Msgf("Failed to subscribe to depth update for symbol: %d", symbolID)
	}
}

func (s *StrategyCommon) SubscribeTick(symbolID int) {
}

// Operations methods
func (s *StrategyCommon) Connect(ctx context.Context) {
	err := s.dataRouter.Connect(ctx)
	if err != nil {
		log().Error().Err(err).Msgf("Failed to connect to data client router")
	}
}

func (s *StrategyCommon) Disconnect() {
	s.dataRouter.Disconnect()
}

// Request methods
func (s *StrategyCommon) ReqDepthSnapshot(symbolID int) error {
	return s.dataRouter.ReqDepthSnapshot(symbolID)
}
