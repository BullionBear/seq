package evbus

import (
	"math"

	"github.com/BullionBear/seq/pkg/model"
)

type RingBuffer struct {
	items   []EventRef
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
		items:   make([]EventRef, actualSize),
		rbRead:  0,
		rbMask:  actualSize - 1,
		rbWrite: 0,
	}
}

// Write writes an item to the ring buffer.
// Returns true if successful, false if the buffer is full.
func (rb *RingBuffer) Write(item EventRef) bool {
	if rb.IsFull() {
		return false
	}
	rb.items[rb.rbWrite&rb.rbMask] = item
	rb.rbWrite++
	return true
}

// Read reads an item from the ring buffer.
// Returns the item and true if successful, false if the buffer is empty.
func (rb *RingBuffer) Read() (EventRef, bool) {
	if rb.IsEmpty() {
		return EventRef{}, false
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
		read:  0,
		write: 0,
		mask:  size - 1,
	}
}

func (a *Arena[T]) Read() T {
	// Mask-based indexing handles overflow automatically:
	// Even if read overflows (after 2^64 operations), read&mask wraps correctly
	item := a.items[a.read&a.mask]
	a.read++
	return item
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

func (e *EventBus) Poll(handler func(event EventRef)) bool {
	if e.rbEvent.IsEmpty() {
		return false
	}
	event, ok := e.rbEvent.Read()
	if !ok {
		return false
	}
	handler(event)
	return false
}

func (e *EventBus) PublishDepthSnapshot(depthSnapshot model.DepthSnapshot) {
	e.arenaDepthSnapshot.Write(depthSnapshot)
}

func (e *EventBus) PublishDepthUpdate(depthUpdate model.DepthUpdate) {
	e.arenaDepthUpdate.Write(depthUpdate)
}

func (e *EventBus) PublishTick(tick model.Tick) {
	e.arenaTick.Write(tick)
}

func (e *EventBus) PublishOrderUpdate(orderUpdate model.OrderUpdate) {
	e.arenaOrderUpdate.Write(orderUpdate)
}

func (e *EventBus) PublishOrderFill(orderFill model.OrderFill) {
	e.arenaOrderFill.Write(orderFill)
}
