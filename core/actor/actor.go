package actor

import (
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/msgbus"
)

// Actor is the unified interface for all event-driven components.
// Both infrastructure actors (OrderBook, EMS) and trading strategies implement this.
//
// Actors are the fundamental building blocks of the trading system:
// - OrderBook actor handles depth snapshots and updates
// - EMS actor handles order updates and fills
// - Strategy actors implement trading logic
type Actor interface {
	// Identity
	// Name returns a unique identifier for this actor.
	// Used for sequence tracking and debugging.
	Name() string

	// Event subscription
	// SubscribedTypes returns the event types this actor wants to receive.
	// Return nil or empty slice to receive all event types.
	SubscribedTypes() []event.Topic

	// Event handling
	// Handle processes an event from the MsgBus.
	// The MsgBus reference is provided to allow reading the full event data
	// from the appropriate arena based on the event reference, and to send
	// commands via the command channel.
	Handle(ev msgbus.Event, bus *msgbus.MsgBus)

	// Command handling
	Process(cmd msgbus.Command, bus *msgbus.MsgBus)

	// Lifecycle methods
	// OnInit is called once when the actor is initialized.
	OnInit()
	// OnStart is called once when the actor is started.
	OnStart()
	// OnStop is called once when the actor is stopped.
	OnStop()
}

// Register is a helper function that registers an Actor with the MsgBus.
// It creates a handler that calls the actor's Handle method with the MsgBus reference.
func Register(bus *msgbus.MsgBus, actor Actor) {
	handler := func(ev msgbus.Event) {
		actor.Handle(ev, bus)
	}
	bus.Register(actor.Name(), actor.SubscribedTypes(), handler)
}
