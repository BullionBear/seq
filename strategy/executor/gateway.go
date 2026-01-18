package executor

type OrderGateway interface {
	SubmitLimitOrder(order Order) error
}
