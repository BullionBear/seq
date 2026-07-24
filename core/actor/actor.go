package actor

import (
	"github.com/BullionBear/seq/core/clock"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
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

	// Lifecycle methods
	// OnInit is called once when the actor is initialized.
	OnInit(map[string]any)
	// OnStart is called once when the actor is started.
	OnStart()
	// OnStop is called once when the actor is stopped.
	OnStop()
}

// InjectClock attaches the MsgBus ticker Clock to the actor when present.
// Safe to call independently of RegisterIn — needed for actors that skip bus
// registration (e.g. risk Guards with nil SubscribedTypes).
func InjectClock(bus *msgbus.MsgBus, a Actor) {
	if t := bus.GetTicker(); t != nil {
		if clk, ok := t.(*clock.Clock); ok {
			type clockSetter interface{ SetClock(*clock.Clock) }
			if cs, ok := a.(clockSetter); ok {
				cs.SetClock(clk)
			}
		}
	}
}

// RegisterIn registers an Actor with the MsgBus at the given dispatch phase.
// It creates a handler that calls the actor's Handle method with the MsgBus
// reference, and injects a Clock if one is attached via SetTicker.
//
// Engines derive the phase from their own type: msgbus.PhaseOf(e.Type()).
// There is no default phase — see docs/CONSUMER_ORDER.md.
func RegisterIn(bus *msgbus.MsgBus, a Actor, phase msgbus.Phase) {
	InjectClock(bus, a)
	handler := func(ev msgbus.Event) {
		a.Handle(ev, bus)
	}
	bus.RegisterPhased(phase, a.Name(), a.SubscribedTypes(), handler)
}

// ApplyName sets the actor's name from the config entry. If name is empty,
// the actor keeps its default name from construction.
// This works on any actor that embeds ActorBase (which provides SetName).
func ApplyName(a Actor, name string) {
	if name == "" {
		return
	}
	type namer interface{ SetName(string) }
	if n, ok := a.(namer); ok {
		n.SetName(name)
	}
}
