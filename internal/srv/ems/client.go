package ems

type ExecutionClient interface {
	SubmitOrder(order *Order) error
	CancelOrder(order *Order) error
}
