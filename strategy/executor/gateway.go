package executor

type OrderGateway struct {
}

func (og *OrderGateway) SubmitOrder(order *Order) error {
	return nil
}

func (og *OrderGateway) SubscribeOrderUpdate(accountID int) {

}

func (og *OrderGateway) SubscribeBalanceUpdate(accountID int) {

}

func (og *OrderGateway) Connect() {

}

func (og *OrderGateway) Close() {

}
