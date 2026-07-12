package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/tradingmode"
)

type stubExecClient struct {
	submitCalls int
	cancelCalls int
}

func (s *stubExecClient) Connect(context.Context) error { return nil }
func (s *stubExecClient) Disconnect()                   {}
func (s *stubExecClient) SubscribeOrderUpdate() error   { return nil }
func (s *stubExecClient) SubscribeFill() error          { return nil }
func (s *stubExecClient) SubscribeBalance() error       { return nil }
func (s *stubExecClient) ReqBalanceSnapshot(common.WalletType) error {
	return nil
}
func (s *stubExecClient) SubmitOrder(int, int, common.Side, common.OrderType, common.TimeInForce, float64, float64) error {
	s.submitCalls++
	return nil
}
func (s *stubExecClient) CancelOrder(int, int, int) error {
	s.cancelCalls++
	return nil
}
func (s *stubExecClient) CancelAllOrders(int) error {
	s.cancelCalls++
	return nil
}

func TestExecutionRouter_PaperBlocksVenueMutations(t *testing.T) {
	r := NewExecutionRouter()
	stub := &stubExecClient{}
	r.RegisterClient(1, stub)

	if r.TradingMode() != tradingmode.ModePaper {
		t.Fatalf("default TradingMode=%q, want paper", r.TradingMode())
	}

	err := r.SubmitOrder(1, 1, 1, common.SideBuy, common.OrderTypeLimit, common.TimeInForceGTC, 1, 1)
	if !errors.Is(err, tradingmode.ErrPaperMode) {
		t.Fatalf("SubmitOrder err=%v, want ErrPaperMode", err)
	}
	err = r.CancelOrder(1, 1, 1, 1)
	if !errors.Is(err, tradingmode.ErrPaperMode) {
		t.Fatalf("CancelOrder err=%v, want ErrPaperMode", err)
	}
	err = r.CancelAllOrders(1, 1)
	if !errors.Is(err, tradingmode.ErrPaperMode) {
		t.Fatalf("CancelAllOrders err=%v, want ErrPaperMode", err)
	}
	if stub.submitCalls != 0 || stub.cancelCalls != 0 {
		t.Fatalf("venue client touched in paper mode: submit=%d cancel=%d", stub.submitCalls, stub.cancelCalls)
	}
}

func TestExecutionRouter_LiveAllowsVenueMutations(t *testing.T) {
	r := NewExecutionRouter()
	r.SetTradingMode(tradingmode.ModeLive)
	stub := &stubExecClient{}
	r.RegisterClient(1, stub)

	if err := r.SubmitOrder(1, 1, 1, common.SideBuy, common.OrderTypeLimit, common.TimeInForceGTC, 1, 1); err != nil {
		t.Fatalf("SubmitOrder: %v", err)
	}
	if err := r.CancelOrder(1, 1, 1, 1); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if err := r.CancelAllOrders(1, 1); err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}
	if stub.submitCalls != 1 || stub.cancelCalls != 2 {
		t.Fatalf("calls submit=%d cancel=%d, want 1 and 2", stub.submitCalls, stub.cancelCalls)
	}
}
