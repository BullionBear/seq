package binance

import "github.com/BullionBear/seq/core/model/common"

type BinanceSpotExecutionClient struct {
}

func NewBinanceSpotExecutionClient() BinanceSpotExecutionClient {
	return BinanceSpotExecutionClient{}
}

func (c *BinanceSpotExecutionClient) CreateOrder(acctID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	return nil
}
