package portfolio

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
)

// Ensure BalanceActor implements the Actor interface
var _ actor.Actor = (*BalanceActor)(nil)

// BalanceActor is an actor owned by the portfolio Engine.
// It subscribes to balance and fill events from the EventBus and routes
// them to the engine's update methods.
type BalanceActor struct {
	actor.ActorBase
	engine *Engine
}

// NewBalanceActor creates a new BalanceActor for the given engine.
func NewBalanceActor(engine *Engine) *BalanceActor {
	return &BalanceActor{
		ActorBase: actor.NewActorBase("portfolio-balance", []event.Topic{
			event.TopicEventBalanceUpdate,
			event.TopicEventReqBalanceSnapshot,
			event.TopicEventFill,
		}),
		engine: engine,
	}
}

// Handle routes events to the engine's update methods.
func (b *BalanceActor) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeBalanceUpdate(buf)
		b.engine.OnBalanceUpdate(update)
	case event.TopicEventReqBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqBalanceSnapshot(buf)
		b.engine.OnReqBalanceSnapshot(snapshot)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		b.engine.OnFill(fill)
	}
}
