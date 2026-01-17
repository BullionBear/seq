package bybit

import "context"

type BybitExecutionClient struct {
}

func NewBybitExecutionClient() *BybitExecutionClient {
	return &BybitExecutionClient{}
}

func (c *BybitExecutionClient) Connect(ctx context.Context) error {
	return nil
}
