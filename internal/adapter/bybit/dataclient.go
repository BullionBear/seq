package bybit

import "context"

type BybitDataClient struct {
	hasSub bool
}

func NewBybitDataClient() BybitDataClient {
	return BybitDataClient{}
}

// HasSub returns true if there are any subscriptions configured
func (c *BybitDataClient) HasSub() bool {
	return c.hasSub
}

func (c *BybitDataClient) SubscribeDepthUpdate(symbolID int) {
	c.hasSub = true
}

func (c *BybitDataClient) SubscribeTrade(symbolID int) {
	c.hasSub = true
}

func (c *BybitDataClient) Connect(ctx context.Context) error {
	return nil
}

func (c *BybitDataClient) Disconnect() {
}
