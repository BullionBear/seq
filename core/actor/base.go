package actor

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

// ActorBase provides a default implementation for the Actor interface.
// Embed this in infrastructure actors like OrderBook, EMS.
// For trading strategies, use StrategyBase instead which provides typed callbacks.
type ActorBase struct {
	name   string
	topics []event.Topic
}

// NewActorBase creates a new ActorBase with the given name and subscribed topics.
func NewActorBase(name string, topics []event.Topic) ActorBase {
	return ActorBase{
		name:   name,
		topics: topics,
	}
}

// Name returns the actor's unique identifier.
func (a *ActorBase) Name() string {
	return a.name
}

// SubscribedTypes returns the topics this actor handles.
func (a *ActorBase) SubscribedTypes() []event.Topic {
	return a.topics
}

// Handle is a no-op default. Concrete actors should override this.
func (a *ActorBase) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	// Default no-op - concrete actors should override
}

// OnInit is a no-op default lifecycle method.
func (a *ActorBase) OnInit(config map[string]any) {}

// OnStart is a no-op default lifecycle method.
func (a *ActorBase) OnStart() {}

// OnStop is a no-op default lifecycle method.
func (a *ActorBase) OnStop() {}
