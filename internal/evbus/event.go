package evbus

import (
	"github.com/BullionBear/seq/core/model/event"
)

type EventRef struct {
	Topic  event.Topic
	Index  uint64 // offset in arena
	Length uint64 // size of data in bytes
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
	Name     string                // unique identifier for this consumer
	Topics   map[event.Topic]bool  // subscribed topics (nil or empty = all topics)
	Handler  EventHandler          // callback function for handling events
	Sequence uint64                // last processed event sequence (EventID)
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
