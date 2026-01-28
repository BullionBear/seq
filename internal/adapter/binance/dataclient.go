package binance

import "context"

type BinanceSpotDataClient struct {
}

func NewBinanceSpotDataClient() BinanceSpotDataClient {
	return BinanceSpotDataClient{}
}

func (c *BinanceSpotDataClient) SubscribeDepthUpdate(symbolID int, options *DepthSubscriptionOptions) {

}

func (c *BinanceSpotDataClient) SubscribeTrade(symbolID int) {
}

func (c *BinanceSpotDataClient) Connect(ctx context.Context) error {
	return nil
}

func (c *BinanceSpotDataClient) Disconnect() {
}
