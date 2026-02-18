package actor

import (
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

// ActorBase provides a default implementation for the Actor interface.
// Embed this in infrastructure actors like OrderBook, EMS.
// For trading strategies, use StrategyBase instead which provides typed callbacks.
type ActorBase struct {
	name   string
	topics []event.Topic
	logger zerolog.Logger
}

// NewActorBase creates a new ActorBase with the given name and subscribed topics.
func NewActorBase(name string, topics []event.Topic) ActorBase {
	return ActorBase{
		name:   name,
		topics: topics,
		logger: logger.Get().With().Str("actor", name).Logger(),
	}
}

// SetName overrides the actor's name and re-initializes its logger with the new name context.
// This is typically called by the engine after construction to apply the name from config.
// If name is empty, this is a no-op.
func (a *ActorBase) SetName(name string) {
	if name == "" {
		return
	}
	a.name = name
	a.logger = logger.Get().With().Str("actor", name).Logger()
}

// Name returns the actor's unique identifier.
func (a *ActorBase) Name() string {
	return a.name
}

// Logger returns the actor's logger.
func (a *ActorBase) Log() *zerolog.Logger {
	return &a.logger
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
