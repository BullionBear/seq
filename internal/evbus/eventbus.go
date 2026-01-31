package evbus

import (
	"encoding/binary"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
)

const (
	// DefaultByteArenaCapacity is the default capacity for the byte arena (1MB)
	DefaultByteArenaCapacity = 1024 * 1024

	// Size constants for fixed-size event types
	sizeOfTick        = unsafe.Sizeof(event.Tick{})
	sizeOfOrderUpdate = unsafe.Sizeof(event.OrderUpdate{})
	sizeOfFill        = unsafe.Sizeof(event.Fill{})
	sizeOfPriceLevel  = unsafe.Sizeof(event.PriceLevel{})

	// Header sizes for variable-size events (without slice data)
	// DepthUpdate: SymbolID(8) + PreviousDepthID(8) + DepthID(8) + CurrentDepthID(8) + NextDepthID(8) + Timestamp(8) = 48 bytes
	// Plus AsksLen(4) + BidsLen(4) = 8 bytes for length prefix
	sizeOfDepthUpdateHeader = 48 + 8

	// DepthSnapshot: SymbolID(8) + DepthID(8) + Timestamp(8) = 24 bytes
	// Plus AsksLen(4) + BidsLen(4) = 8 bytes for length prefix
	sizeOfDepthSnapshotHeader = 24 + 8
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

// PublishTick publishes a Tick event using unsafe pointer casting.
func (e *EventBus) PublishTick(tick event.Tick) {
	size := uint64(sizeOfTick)
	offset := e.byteArena.Reserve(size)

	// Write using unsafe pointer casting
	data := (*[sizeOfTick]byte)(unsafe.Pointer(&tick))[:]
	e.byteArena.WriteAt(offset, data)

	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeTick, Index: offset}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

// ReadTick reads a Tick event using unsafe pointer casting.
func (e *EventBus) ReadTick(offset uint64) event.Tick {
	data := e.byteArena.ReadSlice(offset, uint64(sizeOfTick))
	return *(*event.Tick)(unsafe.Pointer(&data[0]))
}

// PublishOrderUpdate publishes an OrderUpdate event using unsafe pointer casting.
func (e *EventBus) PublishOrderUpdate(orderUpdate event.OrderUpdate) {
	size := uint64(sizeOfOrderUpdate)
	offset := e.byteArena.Reserve(size)

	// Write using unsafe pointer casting
	data := (*[sizeOfOrderUpdate]byte)(unsafe.Pointer(&orderUpdate))[:]
	e.byteArena.WriteAt(offset, data)

	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeOrderUpdate, Index: offset}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

// ReadOrderUpdate reads an OrderUpdate event using unsafe pointer casting.
func (e *EventBus) ReadOrderUpdate(offset uint64) event.OrderUpdate {
	data := e.byteArena.ReadSlice(offset, uint64(sizeOfOrderUpdate))
	return *(*event.OrderUpdate)(unsafe.Pointer(&data[0]))
}

// PublishFill publishes a Fill event using unsafe pointer casting.
func (e *EventBus) PublishFill(fill event.Fill) {
	size := uint64(sizeOfFill)
	offset := e.byteArena.Reserve(size)

	// Write using unsafe pointer casting
	data := (*[sizeOfFill]byte)(unsafe.Pointer(&fill))[:]
	e.byteArena.WriteAt(offset, data)

	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeFill, Index: offset}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

// ReadFill reads a Fill event using unsafe pointer casting.
func (e *EventBus) ReadFill(offset uint64) event.Fill {
	data := e.byteArena.ReadSlice(offset, uint64(sizeOfFill))
	return *(*event.Fill)(unsafe.Pointer(&data[0]))
}

// PublishDepthSnapshot publishes a DepthSnapshot with inline PriceLevel data.
// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
func (e *EventBus) PublishDepthSnapshot(snapshot event.DepthSnapshot) {
	asksLen := uint32(len(snapshot.Asks))
	bidsLen := uint32(len(snapshot.Bids))

	// Calculate total size: header + asks data + bids data
	totalSize := uint64(sizeOfDepthSnapshotHeader) +
		uint64(asksLen)*uint64(sizeOfPriceLevel) +
		uint64(bidsLen)*uint64(sizeOfPriceLevel)

	offset := e.byteArena.Reserve(totalSize)

	// Write header fields using little-endian encoding
	buf := make([]byte, totalSize)
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.SymbolID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.DepthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], snapshot.Timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	// Write Asks inline using unsafe
	for i := range snapshot.Asks {
		priceLevelBytes := (*[sizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Asks[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += int(sizeOfPriceLevel)
	}

	// Write Bids inline using unsafe
	for i := range snapshot.Bids {
		priceLevelBytes := (*[sizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Bids[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += int(sizeOfPriceLevel)
	}

	e.byteArena.WriteAt(offset, buf)

	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeDepthSnapshot, Index: offset}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

// ReadDepthSnapshot reads a DepthSnapshot with slices pointing directly into the buffer.
func (e *EventBus) ReadDepthSnapshot(offset uint64) event.DepthSnapshot {
	// Read header to get lengths
	headerData := e.byteArena.ReadSlice(offset, uint64(sizeOfDepthSnapshotHeader))

	pos := 0
	symbolID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	depthID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	timestamp := binary.LittleEndian.Uint64(headerData[pos:])
	pos += 8

	asksLen := binary.LittleEndian.Uint32(headerData[pos:])
	pos += 4

	bidsLen := binary.LittleEndian.Uint32(headerData[pos:])

	// Calculate offsets for asks and bids data
	asksOffset := offset + uint64(sizeOfDepthSnapshotHeader)
	bidsOffset := asksOffset + uint64(asksLen)*uint64(sizeOfPriceLevel)

	// Create slices pointing directly into the buffer
	var asks []event.PriceLevel
	var bids []event.PriceLevel

	if asksLen > 0 {
		asksData := e.byteArena.ReadSlice(asksOffset, uint64(asksLen)*uint64(sizeOfPriceLevel))
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&asksData[0])), asksLen)
	}

	if bidsLen > 0 {
		bidsData := e.byteArena.ReadSlice(bidsOffset, uint64(bidsLen)*uint64(sizeOfPriceLevel))
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&bidsData[0])), bidsLen)
	}

	return event.DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
}

// PublishDepthUpdate publishes a DepthUpdate with inline PriceLevel data.
// Layout: [SymbolID(8)][PreviousDepthID(8)][DepthID(8)][CurrentDepthID(8)][NextDepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
func (e *EventBus) PublishDepthUpdate(update event.DepthUpdate) {
	asksLen := uint32(len(update.Asks))
	bidsLen := uint32(len(update.Bids))

	// Calculate total size: header + asks data + bids data
	totalSize := uint64(sizeOfDepthUpdateHeader) +
		uint64(asksLen)*uint64(sizeOfPriceLevel) +
		uint64(bidsLen)*uint64(sizeOfPriceLevel)

	offset := e.byteArena.Reserve(totalSize)

	// Write header fields using little-endian encoding
	buf := make([]byte, totalSize)
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.SymbolID))
	pos += 8

	// PreviousDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.PreviousDepthID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.DepthID))
	pos += 8

	// CurrentDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.CurrentDepthID))
	pos += 8

	// NextDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.NextDepthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], update.Timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	// Write Asks inline using unsafe
	for i := range update.Asks {
		priceLevelBytes := (*[sizeOfPriceLevel]byte)(unsafe.Pointer(&update.Asks[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += int(sizeOfPriceLevel)
	}

	// Write Bids inline using unsafe
	for i := range update.Bids {
		priceLevelBytes := (*[sizeOfPriceLevel]byte)(unsafe.Pointer(&update.Bids[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += int(sizeOfPriceLevel)
	}

	e.byteArena.WriteAt(offset, buf)

	now := uint64(time.Now().UnixNano())
	e.rbEvent.Write(Event{Ref: EventRef{DataType: event.DataTypeDepthUpdate, Index: offset}, EventID: e.nextEventID, CreatedAt: now, UpdatedAt: now})
	e.nextEventID++
}

// ReadDepthUpdate reads a DepthUpdate with slices pointing directly into the buffer.
func (e *EventBus) ReadDepthUpdate(offset uint64) event.DepthUpdate {
	// Read header to get lengths
	headerData := e.byteArena.ReadSlice(offset, uint64(sizeOfDepthUpdateHeader))

	pos := 0
	symbolID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	previousDepthID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	depthID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	currentDepthID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	nextDepthID := int(binary.LittleEndian.Uint64(headerData[pos:]))
	pos += 8

	timestamp := binary.LittleEndian.Uint64(headerData[pos:])
	pos += 8

	asksLen := binary.LittleEndian.Uint32(headerData[pos:])
	pos += 4

	bidsLen := binary.LittleEndian.Uint32(headerData[pos:])

	// Calculate offsets for asks and bids data
	asksOffset := offset + uint64(sizeOfDepthUpdateHeader)
	bidsOffset := asksOffset + uint64(asksLen)*uint64(sizeOfPriceLevel)

	// Create slices pointing directly into the buffer
	var asks []event.PriceLevel
	var bids []event.PriceLevel

	if asksLen > 0 {
		asksData := e.byteArena.ReadSlice(asksOffset, uint64(asksLen)*uint64(sizeOfPriceLevel))
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&asksData[0])), asksLen)
	}

	if bidsLen > 0 {
		bidsData := e.byteArena.ReadSlice(bidsOffset, uint64(bidsLen)*uint64(sizeOfPriceLevel))
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&bidsData[0])), bidsLen)
	}

	return event.DepthUpdate{
		SymbolID:        symbolID,
		PreviousDepthID: previousDepthID,
		DepthID:         depthID,
		CurrentDepthID:  currentDepthID,
		NextDepthID:     nextDepthID,
		Timestamp:       timestamp,
		Asks:            asks,
		Bids:            bids,
	}
}

func (e *EventBus) ReadReqDepthSnapshot(offset uint64) event.ReqDepthSnapshot {
	// Read header to get lengths
	return event.ReqDepthSnapshot{
		SymbolID:  0,
		DepthID:   0,
		Timestamp: 0,
		Asks:      nil,
		Bids:      nil,
	}
}
