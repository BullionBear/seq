package integration

type SpotClient interface {
	GetDepth(symbolID int, limit int) (*DepthResponse, error)
	CreateOrder(order *CreateOrderRequest) (int, error)
}

type PerpetualClient interface {
}

type UnifiedClient interface {
}
