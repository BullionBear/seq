package adapter

import "context"

type ExecutionRouter struct {
}

func NewExecutionRouter() *ExecutionRouter {
	return &ExecutionRouter{}
}

func (r *ExecutionRouter) SubscribeOrderUpdate(acctID int) {
}

func (r *ExecutionRouter) SubscribeOrderFill(acctID int) {
}

func (r *ExecutionRouter) Connect(ctx context.Context) error {
	return nil
}

func (r *ExecutionRouter) Disconnect() {
}
