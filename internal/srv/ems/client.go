package ems

type Client interface {
	SubmitOrder(order *Order) error
	CancelOrder(order *Order) error
}
