package binance

type BinanceSpotDataClient struct {
}

func NewBinanceSpotDataClient() *BinanceSpotDataClient {
	return &BinanceSpotDataClient{}
}

func (c *BinanceSpotDataClient) SubscribeDepth(symbolID int, limit int) (unsubscribe func(), err error) {
	return nil, nil
}

func (c *BinanceSpotDataClient) SubscribeTrade(symbolID int) (unsubscribe func(), err error) {
	return nil, nil
}
