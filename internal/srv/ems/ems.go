package ems

import (
	"time"

	"github.com/BullionBear/seq/internal/srv/sms"
	"github.com/BullionBear/seq/pkg/evbus"
)

type ExecutionManager struct {
	sms                *sms.SecretManager
	clientOrderID      int
	activeOrders       map[int]Order  // index by clientOrderID
	client             map[int]Client // acctID to client
	orderUpdateFactory *evbus.EventFactory[OrderUpdate]
	orderFillFactory   *evbus.EventFactory[OrderFill]
}

func NewExecutionManager(sms *sms.SecretManager, orderSize int) *ExecutionManager {
	return &ExecutionManager{
		sms:           sms,
		clientOrderID: 0,
		activeOrders:  make(map[int]Order, orderSize),
		orderUpdateFactory: evbus.NewEventFactory(func(o *OrderUpdate) {
			o.Reset()
		}),
		orderFillFactory: evbus.NewEventFactory(func(f *OrderFill) {
			f.Reset()
		}),
	}
}

func (e *ExecutionManager) MakeLimitOrder(
	strategyID int,
	acctID int,
	symbolID int,
	side Side,
	price float64,
	quantity float64) (int, error) {
	e.clientOrderID++
	order := Order{
		StrategyID:    strategyID,
		ClientOrderID: e.clientOrderID,
		AcctID:        acctID,
		SymbolID:      symbolID,
		Side:          side,
		Status:        StatusInitialized,
		Type:          TypeLimit,
		Price:         price,
		Quantity:      quantity,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	e.activeOrders[e.clientOrderID] = order
	return e.clientOrderID, nil
}

func (e *ExecutionManager) MakeMarketOrder(
	strategyID int,
	acctID int,
	symbolID int,
	side Side,
	quantity float64) (int, error) {
	e.clientOrderID++
	order := Order{
		StrategyID:    strategyID,
		ClientOrderID: e.clientOrderID,
		AcctID:        acctID,
		SymbolID:      symbolID,
		Side:          side,
		Status:        StatusInitialized,
		Type:          TypeMarket,
		Quantity:      quantity,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	e.activeOrders[e.clientOrderID] = order
	return e.clientOrderID, nil
}

func (e *ExecutionManager) SubmitOrder(clientOrderID int) error {
	return nil
}

func (e *ExecutionManager) CancelOrder(clientOrderID int) error {
	return nil
}

func (e *ExecutionManager) SubscribeOrderUpdate(acctID int, callback func(*evbus.Event[OrderUpdate]) error, errCallback func(error)) (unsubscribe func(), err error) {
	return func() {
	}, nil
}

func (e *ExecutionManager) SubscribeOrderFill(acctID int, callback func(*evbus.Event[OrderFill]) error, errCallback func(error)) (unsubscribe func(), err error) {
	return func() {
	}, nil
}
