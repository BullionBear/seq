package ob

import "github.com/BullionBear/seq/strategy"

type OrderBook struct {
	strategy.StrategyCommon
}

func NewOrderBook() *OrderBook {
	return &OrderBook{}
}

func (ob *OrderBook) OnInit(config *strategy.StrategyConfig) error {
	return nil
}

func (ob *OrderBook) OnStart() error {
	return nil
}

func (ob *OrderBook) OnReady() error {
	return nil
}
