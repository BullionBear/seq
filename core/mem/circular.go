package mem

import (
	"github.com/BullionBear/seq/core/logger"
)

var log = logger.Get()

// CircularByteArena is a cache-friendly circular buffer for storing event data.
// It follows the Disruptor pattern with Multi Publisher Single Consumer (MPSC).
// All event data is stored contiguously in a single byte buffer for better cache locality.
type CircularByteArena struct {
	buffer   []byte
	writeOff uint64 // next write position
	readOff  uint64 // consumer's read position (for overwrite detection)
	capacity uint64
}

// NewCircularByteArena creates a new CircularByteArena with the specified capacity.
func NewCircularByteArena(capacity uint64) *CircularByteArena {
	return &CircularByteArena{
		buffer:   make([]byte, capacity),
		writeOff: 0,
		readOff:  0,
		capacity: capacity,
	}
}

// Reserve reserves space in the buffer and returns the offset where data should be written.
// Handles wrap-around: if writeOff + size > capacity, wraps to beginning.
// Handles overwrite: if writing would overwrite unread data, advances readOff and logs warning.
func (a *CircularByteArena) Reserve(size uint64) uint64 {
	// Handle wrap-around: if not enough space at the end, wrap to beginning
	if a.writeOff+size > a.capacity {
		a.writeOff = 0
	}

	offset := a.writeOff

	// Check for overwrite condition
	// This happens when we're about to write over data that hasn't been read yet
	if a.wouldOverwrite(offset, size) {
		log.Warn().
			Uint64("writeOff", a.writeOff).
			Uint64("readOff", a.readOff).
			Uint64("size", size).
			Msg("CircularByteArena: dropping unread data due to buffer overwrite")

		// Advance readOff past the data we're about to overwrite
		a.readOff = offset + size
		if a.readOff >= a.capacity {
			a.readOff = 0
		}
	}

	a.writeOff = offset + size
	return offset
}

// wouldOverwrite checks if writing at offset with given size would overwrite unread data.
func (a *CircularByteArena) wouldOverwrite(offset uint64, size uint64) bool {
	// If readOff is behind writeOff (normal case), no overwrite possible
	// If readOff is ahead of writeOff (wrapped case), check if write range overlaps read position
	if a.readOff <= offset {
		// readOff is behind or at offset, check if we wrap and hit readOff
		return false
	}
	// readOff is ahead of offset, check if write range reaches readOff
	return offset+size > a.readOff
}

// WriteAt writes data to the buffer at the specified offset.
func (a *CircularByteArena) WriteAt(offset uint64, data []byte) {
	copy(a.buffer[offset:offset+uint64(len(data))], data)
}

// ReadSlice returns a slice into the buffer at the given offset with the specified size.
// The returned slice points directly into the buffer - no copy is made.
// The caller must ensure the data is consumed before it's overwritten.
func (a *CircularByteArena) ReadSlice(offset uint64, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// GetSlice returns a writable slice at the given offset for publishers to write data into.
// This should be called after Reserve() to get the buffer slice for serialization.
func (a *CircularByteArena) GetSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// AdvanceRead updates the read position after consumption.
// This should be called by the consumer after processing an event.
func (a *CircularByteArena) AdvanceRead(offset uint64) {
	if offset > a.readOff {
		a.readOff = offset
	}
}

// WriteOff returns the current write offset.
func (a *CircularByteArena) WriteOff() uint64 {
	return a.writeOff
}

// ReadOff returns the current read offset.
func (a *CircularByteArena) ReadOff() uint64 {
	return a.readOff
}

// Capacity returns the buffer capacity.
func (a *CircularByteArena) Capacity() uint64 {
	return a.capacity
}

// Reset resets the arena to its initial state.
func (a *CircularByteArena) Reset() {
	a.writeOff = 0
	a.readOff = 0
}
