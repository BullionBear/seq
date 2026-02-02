package strategy

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
)

// Ensure StrategyBase implements the Actor interface
var _ actor.Actor = (*StrategyBase)(nil)

// StrategyBase provides the foundation for trading strategies.
// It implements the Actor interface and embeds StrategyCommon for
// convenience methods (GetBestBid, SubmitOrder, etc.).
//
// IMPORTANT: Go doesn't have virtual method dispatch, so strategies that embed
// StrategyBase MUST override the Handle() method to dispatch events to their
// own typed callbacks. See XArb for an example:
//
//	func (x *XArb) Handle(ev evbus.Event, bus *evbus.EventBus) {
//	    switch ev.Ref.DataType {
//	    case event.DataTypeDepthUpdate:
//	        buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
//	        x.OnDepthUpdate(evbus.DeserializeDepthUpdate(buf))
//	    // ... etc
//	    }
//	}
type StrategyBase struct {
	*StrategyCommon // Provides service methods: GetBestBid, SubmitOrder, etc.
	name            string
	config          *StrategyConfig
}

// NewStrategyBase creates a new StrategyBase with the given name.
// StrategyCommon is injected later via SetCommon by the Engine.
func NewStrategyBase(name string) *StrategyBase {
	return &StrategyBase{
		name: name,
	}
}

// SetCommon is called by the Engine to inject StrategyCommon.
// This provides access to service methods like GetBestBid, SubmitOrder, etc.
func (s *StrategyBase) SetCommon(common *StrategyCommon) {
	s.StrategyCommon = common
}

// SetConfig is called by the Engine to inject the strategy configuration.
// This is called before OnInit.
func (s *StrategyBase) SetConfig(config *StrategyConfig) {
	s.config = config
}

// Config returns the strategy configuration.
func (s *StrategyBase) Config() *StrategyConfig {
	return s.config
}

// StrategyConfig returns the strategy-specific configuration map.
func (s *StrategyBase) StrategyConfig() map[string]any {
	if s.config == nil {
		return nil
	}
	return s.config.Strategy
}

// ============================================================================
// Actor Interface Implementation
// ============================================================================

// Name returns the strategy's unique identifier.
func (s *StrategyBase) Name() string {
	return s.name
}

// SubscribedTypes returns nil to receive all event types.
// Strategies typically want to see all events for their trading logic.
func (s *StrategyBase) SubscribedTypes() []event.DataType {
	return nil // receive all types
}

// Handle processes an event by dispatching to typed callbacks.
// Override the specific typed callbacks (OnDepthUpdate, OnTick, etc.)
// in your strategy implementation.
func (s *StrategyBase) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		s.OnDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		s.OnDepthUpdate(update)
	case event.DataTypeTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := evbus.DeserializeTick(buf)
		s.OnTick(tick)
	case event.DataTypeOrderUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderUpdate := evbus.DeserializeOrderUpdate(buf)
		s.OnOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		s.OnFill(fill)
		// TODO: Add DataTypeBalanceUpdate when EventBus supports it
	}
}

// ============================================================================
// Lifecycle Methods (override in your strategy)
// ============================================================================

// OnInit is called once when the strategy is initialized.
// Override this to set up subscriptions and initial state.
func (s *StrategyBase) OnInit() {}

// OnStart is called once when the strategy is started.
// Override this to connect to data sources.
func (s *StrategyBase) OnStart() {}

// OnStop is called once when the strategy is stopped.
// Override this to clean up resources.
func (s *StrategyBase) OnStop() {}

// ============================================================================
// Typed Event Callbacks (override in your strategy)
// ============================================================================

// OnDepthSnapshot is called when a depth snapshot is received.
func (s *StrategyBase) OnDepthSnapshot(snapshot event.DepthSnapshot) {}

// OnDepthUpdate is called when a depth update is received.
func (s *StrategyBase) OnDepthUpdate(update event.DepthUpdate) {}

// OnTick is called when a tick is received.
func (s *StrategyBase) OnTick(tick event.Tick) {}

// OnOrderUpdate is called when an order update is received.
func (s *StrategyBase) OnOrderUpdate(update event.OrderUpdate) {}

// OnFill is called when a fill is received.
func (s *StrategyBase) OnFill(fill event.Fill) {}

// OnBalanceUpdate is called when a balance update is received.
func (s *StrategyBase) OnBalanceUpdate(update event.BalanceUpdate) {}
