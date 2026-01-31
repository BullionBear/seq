package evbus

import (
	"time"

	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
)

const (
	// DefaultByteArenaCapacity is the default capacity for the byte arena (1MB)
	DefaultByteArenaCapacity = 1024 * 1024
)

// EventHandler is a function type for handling events
type EventHandler func(event Event)

type RingBuffer struct {
	items   []Event
	rbRead  uint64
	rbMask  uint64
	rbWrite uint64
}

// NewRingBuffer creates a new ring buffer with the specified size.
// Size will be rounded up to the next power of 2 for efficient masking.
func NewRingBuffer(size uint64) RingBuffer {
	// Round up to next power of 2
	actualSize := uint64(1)
	for actualSize < size {
		actualSize <<= 1
	}

	return RingBuffer{
		items:   make([]Event, actualSize),
		rbRead:  0,
		rbMask:  actualSize - 1,
		rbWrite: 0,
	}
}

// Write writes an item to the ring buffer.
// Returns true if successful, false if the buffer is full.
func (rb *RingBuffer) Write(item Event) bool {
	if rb.IsFull() {
		return false
	}
	rb.items[rb.rbWrite&rb.rbMask] = item
	rb.rbWrite++
	return true
}

// Read reads an item from the ring buffer.
// Returns the item and true if successful, false if the buffer is empty.
func (rb *RingBuffer) Read() (Event, bool) {
	if rb.IsEmpty() {
		return Event{}, false
	}
	item := rb.items[rb.rbRead&rb.rbMask]
	rb.rbRead++
	return item, true
}

// IsEmpty returns true if the ring buffer is empty.
func (rb *RingBuffer) IsEmpty() bool {
	return rb.rbRead == rb.rbWrite
}

// IsFull returns true if the ring buffer is full.
func (rb *RingBuffer) IsFull() bool {
	return rb.Count() == uint64(len(rb.items))
}

// Count returns the number of items currently in the ring buffer.
func (rb *RingBuffer) Count() uint64 {
	return rb.rbWrite - rb.rbRead
}

// Size returns the capacity of the ring buffer.
func (rb *RingBuffer) Size() uint64 {
	return uint64(len(rb.items))
}

// Reset clears the ring buffer by resetting read and write pointers.
func (rb *RingBuffer) Reset() {
	rb.rbRead = 0
	rb.rbWrite = 0
}

type EventBus struct {
	nextEventID uint64

	rbEvent RingBuffer

	// Single byte arena for all event data (cache-friendly)
	byteArena *mem.CircularByteArena

	// Multi-consumer support
	consumers   []*Consumer
	minSequence uint64 // minimum sequence across all consumers (for arena release)
}

func NewEventBus() *EventBus {
	return &EventBus{
		nextEventID: 0,
		rbEvent:     NewRingBuffer(4096),
		byteArena:   mem.NewCircularByteArena(DefaultByteArenaCapacity),
		consumers:   make([]*Consumer, 0),
		minSequence: 0,
	}
}

// NewEventBusWithCapacity creates a new EventBus with the specified byte arena capacity.
func NewEventBusWithCapacity(arenaCapacity uint64) *EventBus {
	return &EventBus{
		nextEventID: 0,
		rbEvent:     NewRingBuffer(4096),
		byteArena:   mem.NewCircularByteArena(arenaCapacity),
		consumers:   make([]*Consumer, 0),
		minSequence: 0,
	}
}

// Register adds a consumer to the EventBus with optional type filtering.
// If types is nil or empty, the consumer will receive all event types.
// Consumers should be registered before calling Dispatch.
func (e *EventBus) Register(name string, types []event.DataType, handler EventHandler) {
	consumer := NewConsumer(name, types, handler)
	e.consumers = append(e.consumers, consumer)
}

// Dispatch reads the next event from the ring buffer and dispatches it to all
// consumers whose type filter matches. Returns true if an event was dispatched,
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

	// Dispatch to all consumers whose type filter matches
	for _, consumer := range e.consumers {
		if consumer.ShouldHandle(ev.Ref.DataType) {
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
func (e *EventBus) Publish(ref EventRef) {
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{
		Ref:       ref,
		EventID:   e.nextEventID,
		CreatedAt: now,
		UpdatedAt: now,
	})
	e.nextEventID++
}

// ReadBuffer returns a []byte slice at the given offset/length for consumers
// to deserialize event data from.
func (e *EventBus) ReadBuffer(offset, length uint64) []byte {
	return e.byteArena.ReadSlice(offset, length)
}
