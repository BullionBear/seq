package strategy

import (
	"context"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
)

var log = logger.Get()

type StrategyCommon struct {
	eventBus         *evbus.EventBus
	catalog          *catalog.Catalog
	dataClientRouter *adapter.DataClientRouter
	// executionRouter  adapter.ExecutionRouter
}

func NewStrategyCommon(catalog *catalog.Catalog, eventBus *evbus.EventBus) *StrategyCommon {
	return &StrategyCommon{
		eventBus:         eventBus,
		catalog:          catalog,
		dataClientRouter: adapter.NewDataClientRouter(catalog, eventBus),
	}
}

// Getters
func (s *StrategyCommon) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// Order management methods
func (s *StrategyCommon) SubmitLimitOrder(acctID int, symbolID int, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) int {
	return 0
}

func (s *StrategyCommon) SubmitMarketOrder(acctID int, symbolID int, side common.Side, quantity float64) int {
	return 0
}

func (s *StrategyCommon) CancelOrder(acctID int, symbolID int, clientOrderID int) error {
	return nil
}

func (s *StrategyCommon) GetOrder(acctID int, symbolID int, clientOrderID int) (common.Order, error) {
	return common.Order{}, nil
}

// Private subscription methods
func (s *StrategyCommon) SubscribeOrderUpdate(acctID int) {
}

func (s *StrategyCommon) SubscribeBalanceUpdate(acctID int) {
}

func (s *StrategyCommon) SubscribeOrderFill(acctID int) {
}

// Public subscription methods
func (s *StrategyCommon) SubscribeDepthDelta(symbolID int) {
	err := s.dataClientRouter.SubscribeDepthDelta(symbolID)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to subscribe to depth delta for symbol: %d", symbolID)
	}
}

func (s *StrategyCommon) SubscribeTick(symbolID int) {
}

// Operations methods
func (s *StrategyCommon) Connect(ctx context.Context) {
	err := s.dataClientRouter.Connect(ctx)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to connect to data client router")
	}
}

func (s *StrategyCommon) Disconnect() {
	s.dataClientRouter.Disconnect()
}

// Request methods
func (s *StrategyCommon) ReqDepthSnapshot(symbolID int) error {
	return s.dataClientRouter.ReqDepthSnapshot(symbolID)
}

// virtual methods
func (s *StrategyCommon) OnDepthUpdate(depthUpdate event.DepthUpdate) {
}

func (s *StrategyCommon) OnTick(tick event.Tick) {
}

func (s *StrategyCommon) OnOrderUpdate(orderUpdate event.OrderUpdate) {
}

func (s *StrategyCommon) OnFill(fill event.Fill) {
}

func (s *StrategyCommon) OnBalanceUpdate(balanceUpdate event.BalanceUpdate) {
}

func (s *StrategyCommon) OnReqDepthSnapshot(depthSnapshot event.DepthSnapshot) {
}
