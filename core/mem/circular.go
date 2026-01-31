package mem

import (
	"sync/atomic"

	"github.com/BullionBear/seq/core/logger"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// CircularByteArena is a lock-free circular buffer for storing variable-sized event data.
// It follows the MPSC (Multiple Producer Single Consumer) pattern using atomic CAS operations.
//
// All event data is stored contiguously in a single byte buffer for better cache locality.
// Each allocation is guaranteed to be contiguous (no splitting across wrap boundary).
//
// Thread safety:
//   - Reserve can be called from multiple goroutines concurrently (multiple producers)
//   - GetSlice can be called concurrently after Reserve (each producer writes to its own slice)
//   - ReadSlice should be called from a single goroutine (single consumer)
//   - AdvanceRead should be called from a single goroutine (single consumer)
//
// Usage pattern:
//  1. Producer calls Reserve(size) to atomically claim space
//  2. Producer calls GetSlice(offset, size) to get writable buffer
//  3. Producer writes data to the buffer
//  4. Producer publishes event with offset/size (via EventBus.Publish)
//  5. Consumer receives event and calls ReadSlice(offset, size) to read data
type CircularByteArena struct {
	buffer   []byte
	capacity uint64
	_pad0    [48]byte // Padding to prevent false sharing
	writeOff uint64   // next write position (producers compete for this via CAS)
	_pad1    [56]byte
	readOff  uint64 // consumer's read position (for overwrite detection)
	_pad2    [56]byte
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

// Reserve atomically reserves space in the buffer and returns the offset where data should be written.
// This method is thread-safe and can be called from multiple goroutines concurrently.
//
// Handles wrap-around: if writeOff + size > capacity, wraps to beginning.
// Handles overwrite: if writing would overwrite unread data, logs a warning (best effort).
//
// Note: Allocations larger than capacity will cause infinite loop - caller must ensure size <= capacity.
func (a *CircularByteArena) Reserve(size uint64) uint64 {
	for {
		writeOff := atomic.LoadUint64(&a.writeOff)

		newWriteOff := writeOff + size
		actualOffset := writeOff

		// Check for wrap-around: if not enough space at the end, wrap to beginning
		if newWriteOff > a.capacity {
			// Wrap to beginning
			actualOffset = 0
			newWriteOff = size
		}

		// Try to atomically claim this space
		if atomic.CompareAndSwapUint64(&a.writeOff, writeOff, newWriteOff) {
			// Successfully claimed [actualOffset, actualOffset+size)
			// Check for overwrite condition (advisory - best effort under concurrency)
			a.checkOverwrite(actualOffset, size)
			return actualOffset
		}
		// CAS failed, another producer claimed the space, retry
	}
}

// checkOverwrite checks if the reserved space would overwrite unread data.
// This is advisory and best-effort under high concurrency.
func (a *CircularByteArena) checkOverwrite(offset uint64, size uint64) {
	readOff := atomic.LoadUint64(&a.readOff)

	// Check if we're overwriting unread data
	// This happens when readOff is ahead of our offset (wrapped case)
	// and our write range reaches or passes readOff
	if readOff > offset && offset+size > readOff {
		log().Warn().
			Uint64("writeOffset", offset).
			Uint64("readOff", readOff).
			Uint64("size", size).
			Msg("CircularByteArena: potential overwrite of unread data")

		// Advance readOff past the data we're about to overwrite
		// Use CAS to avoid race with consumer's AdvanceRead
		newReadOff := offset + size
		if newReadOff >= a.capacity {
			newReadOff = 0
		}
		// Best effort - don't retry if CAS fails
		atomic.CompareAndSwapUint64(&a.readOff, readOff, newReadOff)
	}
}

// WriteAt writes data to the buffer at the specified offset.
// This is safe to call concurrently as long as different producers write to different offsets
// (which is guaranteed when using offsets from Reserve).
func (a *CircularByteArena) WriteAt(offset uint64, data []byte) {
	copy(a.buffer[offset:offset+uint64(len(data))], data)
}

// ReadSlice returns a slice into the buffer at the given offset with the specified size.
// The returned slice points directly into the buffer - no copy is made.
// The caller must ensure the data is consumed before it's overwritten.
//
// This should be called from a single goroutine (single consumer).
func (a *CircularByteArena) ReadSlice(offset uint64, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// GetSlice returns a writable slice at the given offset for publishers to write data into.
// This should be called after Reserve() to get the buffer slice for serialization.
//
// This is safe to call concurrently as long as different producers use different offsets
// (which is guaranteed when using offsets from Reserve).
func (a *CircularByteArena) GetSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// AdvanceRead updates the read position after consumption.
// This should be called by the consumer after processing an event.
//
// This should be called from a single goroutine (single consumer).
func (a *CircularByteArena) AdvanceRead(offset uint64) {
	readOff := atomic.LoadUint64(&a.readOff)
	if offset > readOff {
		atomic.StoreUint64(&a.readOff, offset)
	}
}

// WriteOff returns the current write offset.
func (a *CircularByteArena) WriteOff() uint64 {
	return atomic.LoadUint64(&a.writeOff)
}

// ReadOff returns the current read offset.
func (a *CircularByteArena) ReadOff() uint64 {
	return atomic.LoadUint64(&a.readOff)
}

// Capacity returns the buffer capacity.
func (a *CircularByteArena) Capacity() uint64 {
	return a.capacity
}

// Reset resets the arena to its initial state.
// WARNING: This is NOT thread-safe and should only be called when no operations are in progress.
func (a *CircularByteArena) Reset() {
	atomic.StoreUint64(&a.writeOff, 0)
	atomic.StoreUint64(&a.readOff, 0)
}
