package bybit

type BybitDataClient struct {
}

func NewBybitDataClient() *BybitDataClient {
	return &BybitDataClient{}
}

func (c *BybitDataClient) SubscribeDepth(symbolID int, limit int) (unsubscribe func(), err error) {
	return nil, nil
}

func (c *BybitDataClient) SubscribeTrade(symbolID int) (unsubscribe func(), err error) {
	return nil, nil
}
