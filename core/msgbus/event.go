package msgbus

import (
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
)

// EventRef is a reference to event data stored in the event arena.
// EventRefs are created by EventBus.Allocate, which also records the arena
// reservation so the space can be returned after dispatch. Length may be
// shrunk by the producer before Publish if less data was written than
// allocated; the full reservation is still released.
type EventRef struct {
	Topic  event.Topic
	Index  uint64 // offset in arena
	Length uint64 // size of data in bytes

	// Arena reservation range (monotonic), set by Allocate.
	resStart uint64
	resEnd   uint64
}

// reservation reconstructs the arena reservation for release.
func (r EventRef) reservation() mem.Reservation {
	return mem.Reservation{Start: r.resStart, End: r.resEnd, Offset: r.Index}
}

// Event wraps data with metadata. Data is embedded as a value type
// so Event and Data are pooled together (single allocation).
type Event struct {
	Ref       EventRef
	EventID   uint64
	CreatedAt uint64
	UpdatedAt uint64
}

// Consumer represents a subscriber to the EventBus with optional topic filtering.
// Each consumer tracks its own sequence (last processed event ID) for
// coordinating arena memory release across multiple consumers.
type Consumer struct {
	Name     string               // unique identifier for this consumer
	Phase    Phase                // dispatch phase; see docs/CONSUMER_ORDER.md
	Topics   map[event.Topic]bool // subscribed topics (nil or empty = all topics)
	Handler  EventHandler         // callback function for handling events
	Sequence uint64               // last processed event sequence (EventID)
}

// NewConsumer creates a new consumer with the given name, subscribed topics, and handler.
// If topics is nil or empty, the consumer will receive all event types.
func NewConsumer(name string, topics []event.Topic, handler EventHandler) *Consumer {
	topicMap := make(map[event.Topic]bool)
	for _, t := range topics {
		topicMap[t] = true
	}
	return &Consumer{
		Name:     name,
		Topics:   topicMap,
		Handler:  handler,
		Sequence: 0,
	}
}

// ShouldHandle returns true if this consumer should handle the given topic.
// Returns true for all topics if the consumer has no topic filter (empty Topics map).
func (c *Consumer) ShouldHandle(topic event.Topic) bool {
	if len(c.Topics) == 0 {
		return true // no filter, handle all topics
	}
	return c.Topics[topic]
}
