package bybit

import "context"

type BybitDataClient struct {
}

func NewBybitDataClient() BybitDataClient {
	return BybitDataClient{}
}

func (c *BybitDataClient) SubscribeDepthUpdate(symbolID int) {
}

func (c *BybitDataClient) SubscribeTrade(symbolID int) {
}

func (c *BybitDataClient) Connect(ctx context.Context) error {
	return nil
}

func (c *BybitDataClient) Disconnect() {
}
