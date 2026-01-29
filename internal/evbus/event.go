package evbus

import (
	"github.com/BullionBear/seq/core/model/event"
)

type EventRef struct {
	DataType event.DataType
	Index    uint64
}

// Event wraps data with metadata. Data is embedded as a value type
// so Event and Data are pooled together (single allocation).
type Event struct {
	Ref       EventRef
	EventID   uint64
	CreatedAt uint64
	UpdatedAt uint64
}

// Consumer represents a subscriber to the EventBus with optional type filtering.
// Each consumer tracks its own sequence (last processed event ID) for
// coordinating arena memory release across multiple consumers.
type Consumer struct {
	Name     string                   // unique identifier for this consumer
	Types    map[event.DataType]bool  // subscribed types (nil or empty = all types)
	Handler  EventHandler             // callback function for handling events
	Sequence uint64                   // last processed event sequence (EventID)
}

// NewConsumer creates a new consumer with the given name, subscribed types, and handler.
// If types is nil or empty, the consumer will receive all event types.
func NewConsumer(name string, types []event.DataType, handler EventHandler) *Consumer {
	typeMap := make(map[event.DataType]bool)
	for _, t := range types {
		typeMap[t] = true
	}
	return &Consumer{
		Name:     name,
		Types:    typeMap,
		Handler:  handler,
		Sequence: 0,
	}
}

// ShouldHandle returns true if this consumer should handle the given event type.
// Returns true for all types if the consumer has no type filter (empty Types map).
func (c *Consumer) ShouldHandle(dataType event.DataType) bool {
	if len(c.Types) == 0 {
		return true // no filter, handle all types
	}
	return c.Types[dataType]
}
