package evbus

import (
	"math"
	"time"

	"github.com/BullionBear/seq/pkg/model"
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
	items []T
	read  uint64
	write uint64
	mask  uint64
}

func NewArena[T any](size uint64) Arena[T] {
	// If size is not a power of 2, round up to the next power of 2
	if size&(size-1) != 0 {
		size = 1 << uint64(math.Ceil(math.Log2(float64(size))))
	}
	return Arena[T]{
		items: make([]T, size), // Allocate actual array
		write: 0,
		mask:  size - 1,
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

type EventBus struct {
	nextEventID uint64

	rbEvent RingBuffer

	arenaDepthSnapshot Arena[model.DepthSnapshot]
	arenaDepthUpdate   Arena[model.DepthUpdate]
	arenaTick          Arena[model.Tick]
	arenaOrderUpdate   Arena[model.OrderUpdate]
	arenaOrderFill     Arena[model.OrderFill]

	arenaPriceLevels Arena[model.PriceLevel]
}

func NewEventBus() EventBus {
	return EventBus{
		nextEventID:        0,
		rbEvent:            NewRingBuffer(4096),
		arenaDepthSnapshot: NewArena[model.DepthSnapshot](16),
		arenaDepthUpdate:   NewArena[model.DepthUpdate](2048),
		arenaTick:          NewArena[model.Tick](2048),
		arenaOrderUpdate:   NewArena[model.OrderUpdate](1024),
		arenaOrderFill:     NewArena[model.OrderFill](1024),
		arenaPriceLevels:   NewArena[model.PriceLevel](4096),
	}
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

func (e *EventBus) PublishDepthSnapshot(depthSnapshot model.DepthSnapshot) {
	idx := e.arenaDepthSnapshot.Write(depthSnapshot)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: model.DataTypeDepthSnapshot, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadDepthSnapshot(index uint64) model.DepthSnapshot {
	return e.arenaDepthSnapshot.Read(index)
}

func (e *EventBus) PublishDepthUpdate(depthUpdate model.DepthUpdate) {
	idx := e.arenaDepthUpdate.Write(depthUpdate)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: model.DataTypeDepthUpdate, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadDepthUpdate(index uint64) model.DepthUpdate {
	return e.arenaDepthUpdate.Read(index)
}

func (e *EventBus) PublishTick(tick model.Tick) {
	idx := e.arenaTick.Write(tick)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: model.DataTypeTick, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadTick(index uint64) model.Tick {
	return e.arenaTick.Read(index)
}

func (e *EventBus) PublishOrderUpdate(orderUpdate model.OrderUpdate) {
	idx := e.arenaOrderUpdate.Write(orderUpdate)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: model.DataTypeOrderUpdate, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadOrderUpdate(index uint64) model.OrderUpdate {
	return e.arenaOrderUpdate.Read(index)
}

func (e *EventBus) PublishOrderFill(orderFill model.OrderFill) {
	idx := e.arenaOrderFill.Write(orderFill)
	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: model.DataTypeOrderFill, Index: idx}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

func (e *EventBus) ReadOrderFill(index uint64) model.OrderFill {
	return e.arenaOrderFill.Read(index)
}
