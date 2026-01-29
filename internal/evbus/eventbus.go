package evbus

import (
	"math"
	"time"

	"github.com/BullionBear/seq/core/model/event"
)

const (
	MaxDepthLevels = 2048
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

type Arena[T any] struct {
	items   []T
	read    uint64
	write   uint64
	mask    uint64
	minRead uint64 // minimum read position across all consumers
}

func NewArena[T any](size uint64) Arena[T] {
	// If size is not a power of 2, round up to the next power of 2
	if size&(size-1) != 0 {
		size = 1 << uint64(math.Ceil(math.Log2(float64(size))))
	}
	return Arena[T]{
		items:   make([]T, size), // Allocate actual array
		write:   0,
		mask:    size - 1,
		minRead: 0,
	}
}

func (a *Arena[T]) Read(index uint64) T {
	// Mask-based indexing handles overflow automatically:
	// Even if read overflows (after 2^64 operations), read&mask wraps correctly
	return a.items[index&a.mask]
}

func (a *Arena[T]) Write(item T) uint64 {
	index := a.write & a.mask
	a.items[index] = item
	a.write++
	return index
}

// CanWrite returns true if writing would not overwrite unprocessed data.
// This checks if the write position has wrapped around to data that
// hasn't been released by all consumers yet.
func (a *Arena[T]) CanWrite() bool {
	// Can write if we haven't wrapped around to minRead
	return a.write-a.minRead < uint64(len(a.items))
}

// Release updates the minimum read position, indicating that all consumers
// have processed up to this index. Arena slots before this can be safely overwritten.
func (a *Arena[T]) Release(upTo uint64) {
	if upTo > a.minRead {
		a.minRead = upTo
	}
}

// MinRead returns the minimum read position across all consumers.
func (a *Arena[T]) MinRead() uint64 {
	return a.minRead
}

// Size returns the capacity of the arena.
func (a *Arena[T]) Size() uint64 {
	return uint64(len(a.items))
}

type EventBus struct {
	nextEventID uint64

	rbEvent RingBuffer

	arenaDepthSnapshot Arena[event.DepthSnapshot]
	arenaDepthUpdate   Arena[event.DepthUpdate]
	arenaTick          Arena[event.Tick]
	arenaOrderUpdate   Arena[event.OrderUpdate]
	arenaFill          Arena[event.Fill]

	PriceLevels [MaxDepthLevels]event.PriceLevel
	offset      int

	// Multi-consumer support
	consumers   []*Consumer
	minSequence uint64 // minimum sequence across all consumers (for arena release)
}

func NewEventBus() *EventBus {
	return &EventBus{
		nextEventID:        0,
		rbEvent:            NewRingBuffer(4096),
		arenaDepthSnapshot: NewArena[event.DepthSnapshot](16),
		arenaDepthUpdate:   NewArena[event.DepthUpdate](2048),
		arenaTick:          NewArena[event.Tick](2048),
		arenaOrderUpdate:   NewArena[event.OrderUpdate](1024),
		arenaFill:          NewArena[event.Fill](1024),

		PriceLevels: [MaxDepthLevels]event.PriceLevel{},
		offset:      0,

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

// ReleaseArenas releases memory in all arenas up to the current minSequence.
// This should be called after Release() to propagate the sequence to all arenas.
// Note: Arena release is based on the event index stored in each arena, not the
// global event sequence. This is a simplified implementation that releases based
// on minSequence as a proxy for time progression.
func (e *EventBus) ReleaseArenas() {
	e.arenaDepthSnapshot.Release(e.minSequence)
	e.arenaDepthUpdate.Release(e.minSequence)
	e.arenaTick.Release(e.minSequence)
	e.arenaOrderUpdate.Release(e.minSequence)
	e.arenaFill.Release(e.minSequence)
}

// ConsumerCount returns the number of registered consumers.
func (e *EventBus) ConsumerCount() int {
	return len(e.consumers)
}

func (e *EventBus) AllocPriceLevels(size int) []event.PriceLevel {
	offset := e.offset
	e.offset += size
	if e.offset >= MaxDepthLevels {
		offset = 0
		e.offset = size
	}
	return e.PriceLevels[offset:e.offset]
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

func (e *EventBus) PublishDepthSnapshot(depthSnapshot event.DepthSnapshot) {
	idx := e.arenaDepthSnapshot.Write(depthSnapshot)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeDepthSnapshot, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadDepthSnapshot(index uint64) event.DepthSnapshot {
	return e.arenaDepthSnapshot.Read(index)
}

func (e *EventBus) PublishDepthUpdate(depthUpdate event.DepthUpdate) {
	idx := e.arenaDepthUpdate.Write(depthUpdate)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeDepthUpdate, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadDepthUpdate(index uint64) event.DepthUpdate {
	return e.arenaDepthUpdate.Read(index)
}

func (e *EventBus) PublishTick(tick event.Tick) {
	idx := e.arenaTick.Write(tick)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeTick, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadTick(index uint64) event.Tick {
	return e.arenaTick.Read(index)
}

func (e *EventBus) PublishOrderUpdate(orderUpdate event.OrderUpdate) {
	idx := e.arenaOrderUpdate.Write(orderUpdate)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeOrderUpdate, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadOrderUpdate(index uint64) event.OrderUpdate {
	return e.arenaOrderUpdate.Read(index)
}

func (e *EventBus) PublishFill(fill event.Fill) {
	idx := e.arenaFill.Write(fill)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeFill, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadFill(index uint64) event.Fill {
	return e.arenaFill.Read(index)
}
