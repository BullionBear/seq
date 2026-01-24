package strategy

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/rs/zerolog"
)

type StrategyCommon struct {
	log              zerolog.Logger
	dataClientRouter adapter.DataClientRouter
	executionRouter  adapter.ExecutionRouter
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

func (s *StrategyCommon) UnsubscribeOrderUpdate(acctID int) {
}

func (s *StrategyCommon) UnsubscribeBalanceUpdate(acctID int) {
}

func (s *StrategyCommon) UnsubscribeOrderFill(acctID int) {
}

// Public subscription methods
func (s *StrategyCommon) SubscribeDepthDelta(symbolID int) {
}

func (s *StrategyCommon) SubscribeTick(symbolID int) {
}

func (s *StrategyCommon) UnsubscribeDepthDelta(symbolID int) {
}

func (s *StrategyCommon) UnsubscribeTick(symbolID int) {
}

// virtual methods
func (s *StrategyCommon) OnDepthUpdate(depthUpdate event.DepthUpdate) error {
	return nil
}

func (s *StrategyCommon) OnTick(tick event.Tick) error {
	return nil
}

func (s *StrategyCommon) OnOrderUpdate(orderUpdate event.OrderUpdate) error {
	return nil
}

func (s *StrategyCommon) OnFill(fill event.Fill) error {
	return nil
}

func (s *StrategyCommon) OnBalanceUpdate(balanceUpdate event.BalanceUpdate) error {
	return nil
}
