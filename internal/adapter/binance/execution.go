package binance

import "github.com/BullionBear/seq/pkg/model"

type BinanceSpotExecutionClient struct {
}

func NewBinanceSpotExecutionClient() BinanceSpotExecutionClient {
	return BinanceSpotExecutionClient{}
}

func (c *BinanceSpotExecutionClient) CreateOrder(acctID int, symbolID int, side model.Side, orderType model.OrderType, timeInForce model.TimeInForce, price float64, quantity float64) error {
	return nil
}
