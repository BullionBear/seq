package evbus

import (
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
)

const (
	// DefaultByteArenaCapacity is the default capacity for the byte arena (1MB)
	DefaultByteArenaCapacity = 1024 * 1024

	// DefaultEventRingBufferSize is the default size for the event ring buffer
	DefaultEventRingBufferSize = 4096
)

// EventHandler is a function type for handling events
type EventHandler func(event Event)

// EventBus is the central event distribution system.
// It uses a lock-free MPSC (Multiple Producer Single Consumer) ring buffer
// for events, allowing multiple goroutines to publish events concurrently
// while a single consumer (the dispatch loop) processes them.
//
// Thread safety:
//   - Publish can be called from multiple goroutines concurrently
//   - Dispatch should be called from a single goroutine
//   - Allocate is NOT thread-safe (arena access should be serialized or use per-producer arenas)
type EventBus struct {
	nextEventID uint64

	// MPSC ring buffer for events - supports concurrent producers
	rbEvent *mem.MPSCRingBuffer[Event]

	// Single byte arena for all event data (cache-friendly)
	byteArena *mem.CircularByteArena

	// Multi-consumer support
	consumers   []*Consumer
	minSequence uint64 // minimum sequence across all consumers (for arena release)
}

// NewEventBus creates a new EventBus with default capacity.
func NewEventBus() *EventBus {
	return &EventBus{
		nextEventID: 0,
		rbEvent:     mem.NewMPSCRingBuffer[Event](DefaultEventRingBufferSize),
		byteArena:   mem.NewCircularByteArena(DefaultByteArenaCapacity),
		consumers:   make([]*Consumer, 0),
		minSequence: 0,
	}
}

// NewEventBusWithCapacity creates a new EventBus with the specified byte arena capacity.
func NewEventBusWithCapacity(arenaCapacity uint64) *EventBus {
	return &EventBus{
		nextEventID: 0,
		rbEvent:     mem.NewMPSCRingBuffer[Event](DefaultEventRingBufferSize),
		byteArena:   mem.NewCircularByteArena(arenaCapacity),
		consumers:   make([]*Consumer, 0),
		minSequence: 0,
	}
}

// Register adds a consumer to the EventBus with optional topic filtering.
// If topics is nil or empty, the consumer will receive all topics.
// Consumers should be registered before calling Dispatch.
func (e *EventBus) Register(name string, topics []event.Topic, handler EventHandler) {
	consumer := NewConsumer(name, topics, handler)
	e.consumers = append(e.consumers, consumer)
}

// Dispatch reads the next event from the ring buffer and dispatches it to all
// consumers whose topic filter matches. Returns true if an event was dispatched,
// false if the ring buffer is empty.
// After all consumers process the event, their sequences are updated.
func (e *EventBus) Dispatch() bool {
	if e.rbEvent.IsEmpty() {
		return false
	}
	ev, ok := e.rbEvent.Read()
	if !ok {
		return false
	}

	// Dispatch to all consumers whose topic filter matches
	for _, consumer := range e.consumers {
		if consumer.ShouldHandle(ev.Ref.Topic) {
			consumer.Handler(ev)
		}
		// Update consumer sequence regardless of whether it handled the event
		// This ensures minSequence advances correctly
		consumer.Sequence = ev.EventID
	}

	ev.UpdatedAt = uint64(time.Now().UnixNano())
	return true
}

// Release updates minSequence to the minimum sequence across all consumers.
// This indicates the oldest event that is still being processed.
// Arena memory up to minSequence can be safely reclaimed.
func (e *EventBus) Release() {
	if len(e.consumers) == 0 {
		return
	}

	min := e.consumers[0].Sequence
	for _, consumer := range e.consumers[1:] {
		if consumer.Sequence < min {
			min = consumer.Sequence
		}
	}
	e.minSequence = min
}

// MinSequence returns the minimum sequence across all consumers.
// This can be used to determine which arena slots are safe to overwrite.
func (e *EventBus) MinSequence() uint64 {
	return e.minSequence
}

// ReleaseArenas is kept for backward compatibility.
// With CircularByteArena, memory management is handled automatically
// through write-time overwrite detection.
func (e *EventBus) ReleaseArenas() {
	// No-op: CircularByteArena handles overwrite detection internally
}

// ConsumerCount returns the number of registered consumers.
func (e *EventBus) ConsumerCount() int {
	return len(e.consumers)
}

func (e *EventBus) Poll(handler EventHandler) bool {
	if e.rbEvent.IsEmpty() {
		return false
	}
	event, ok := e.rbEvent.Read()
	if !ok {
		return false
	}
	handler(event)
	event.UpdatedAt = uint64(time.Now().UnixNano())
	// logging the event (TBD)
	return true
}

// Allocate reserves space in the arena and returns the offset and a []byte slice
// for the caller to write data into. The caller is responsible for serializing
// data into the returned buffer before calling Publish.
func (e *EventBus) Allocate(size uint64) (offset uint64, buffer []byte) {
	offset = e.byteArena.Reserve(size)
	buffer = e.byteArena.GetSlice(offset, size)
	return offset, buffer
}

// Publish publishes an EventRef to the ring buffer. The caller should have
// already serialized data into the arena buffer obtained via Allocate.
// This method is thread-safe and can be called from multiple goroutines.
func (e *EventBus) Publish(ref EventRef) {
	now := uint64(time.Now().UnixNano())
	// Use atomic increment to get unique event ID for concurrent publishers
	eventID := atomic.AddUint64(&e.nextEventID, 1) - 1
	e.rbEvent.Write(Event{
		Ref:       ref,
		EventID:   eventID,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// ReadBuffer returns a []byte slice at the given offset/length for consumers
// to deserialize event data from.
func (e *EventBus) ReadBuffer(offset, length uint64) []byte {
	return e.byteArena.ReadSlice(offset, length)
}
