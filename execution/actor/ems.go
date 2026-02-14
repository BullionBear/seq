package actor

import (
	coreactor "github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/msgbus"
)

// EMSEngineHandler is implemented by the execution engine.
// It receives order and fill event callbacks from EMS.
type EMSEngineHandler interface {
	OnOrderAccepted(ev event.OrderAccepted)
	OnOrderPartiallyFilled(ev event.OrderPartiallyFilled)
	OnFill(ev event.Fill)
	OnOrderCanceled(ev event.OrderCanceled)
	OnOrderRejected(ev event.OrderRejected)
}

// Ensure EMS implements the Actor interface
var _ coreactor.Actor = (*EMS)(nil)

// EMS (Execution Management System) is an actor owned by the execution Engine.
// It subscribes to order and fill events from the EventBus and routes them to
// the engine's update methods, which maintain open order state.
type EMS struct {
	coreactor.ActorBase
	engine EMSEngineHandler
}

// NewEMS creates a new EMS actor for the given engine handler.
func NewEMS(engine EMSEngineHandler) *EMS {
	return &EMS{
		ActorBase: coreactor.NewActorBase("ems", []event.Topic{
			event.TopicEventOrderAccepted,
			event.TopicEventPartialFill,
			event.TopicEventFill,
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
		}),
		engine: engine,
	}
}

// Handle routes order and fill events to the engine's update methods.
func (e *EMS) Handle(ev evbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventOrderAccepted:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		e.engine.OnOrderAccepted(evbus.DeserializeOrderAccepted(buf))
	case event.TopicEventPartialFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		e.engine.OnOrderPartiallyFilled(evbus.DeserializeOrderPartiallyFilled(buf))
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		e.engine.OnFill(evbus.DeserializeFill(buf))
	case event.TopicEventOrderCanceled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		e.engine.OnOrderCanceled(evbus.DeserializeOrderCanceled(buf))
	case event.TopicEventOrderRejected:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		e.engine.OnOrderRejected(evbus.DeserializeOrderRejected(buf))
	}
}
