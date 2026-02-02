package actor

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
)

// ActorBase provides a default implementation for the Actor interface.
// Embed this in infrastructure actors like OrderBook, EMS.
// For trading strategies, use StrategyBase instead which provides typed callbacks.
type ActorBase struct {
	name  string
	types []event.DataType
}

// NewActorBase creates a new ActorBase with the given name and subscribed types.
func NewActorBase(name string, types []event.DataType) ActorBase {
	return ActorBase{
		name:  name,
		types: types,
	}
}

// Name returns the actor's unique identifier.
func (a *ActorBase) Name() string {
	return a.name
}

// SubscribedTypes returns the event types this actor handles.
func (a *ActorBase) SubscribedTypes() []event.DataType {
	return a.types
}

// Handle is a no-op default. Concrete actors should override this.
func (a *ActorBase) Handle(ev evbus.Event, bus *evbus.EventBus) {
	// Default no-op - concrete actors should override
}

// OnInit is a no-op default lifecycle method.
func (a *ActorBase) OnInit() {}

// OnStart is a no-op default lifecycle method.
func (a *ActorBase) OnStart() {}

// OnStop is a no-op default lifecycle method.
func (a *ActorBase) OnStop() {}
