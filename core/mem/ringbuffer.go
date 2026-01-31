package mem

import (
	"sync/atomic"
)

// nextPowerOf2 rounds up to the next power of 2
func nextPowerOf2(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}

// =============================================================================
// SPSC Ring Buffer - Single Producer Single Consumer
// =============================================================================

// SPSCRingBuffer is a lock-free Single Producer Single Consumer ring buffer.
// It uses atomic operations for proper memory ordering between producer and consumer threads.
//
// Performance characteristics:
//   - ~10-20ns per operation with no contention
//   - Cache-line padding prevents false sharing
//   - Uses monotonically increasing counters (no wrap-around issues)
//
// Thread safety:
//   - Exactly ONE goroutine should call Write (producer)
//   - Exactly ONE goroutine should call Read (consumer)
//   - IsEmpty, IsFull, Count, Capacity are safe to call from any goroutine
type SPSCRingBuffer[T any] struct {
	items []T
	mask  uint64
	_pad0 [56]byte // Padding to prevent false sharing (cache line = 64 bytes)
	write uint64   // Producer writes here
	_pad1 [56]byte
	read  uint64 // Consumer reads here
	_pad2 [56]byte
}

// NewSPSCRingBuffer creates a new SPSC ring buffer with the specified size.
// Size will be rounded up to the next power of 2 for efficient masking.
func NewSPSCRingBuffer[T any](size uint64) *SPSCRingBuffer[T] {
	actualSize := nextPowerOf2(size)
	return &SPSCRingBuffer[T]{
		items: make([]T, actualSize),
		mask:  actualSize - 1,
		write: 0,
		read:  0,
	}
}

// Write writes an item to the ring buffer.
// Returns true if successful, false if the buffer is full.
// Only one goroutine should call Write (single producer).
func (rb *SPSCRingBuffer[T]) Write(item T) bool {
	write := atomic.LoadUint64(&rb.write)
	read := atomic.LoadUint64(&rb.read)

	// Check if full
	if write-read >= uint64(len(rb.items)) {
		return false
	}

	// Write the item first
	rb.items[write&rb.mask] = item

	// Memory barrier: ensure item is written before write pointer is updated
	// This guarantees the consumer sees the item when it sees the updated write pointer
	atomic.StoreUint64(&rb.write, write+1)
	return true
}

// Read reads an item from the ring buffer.
// Returns the item and true if successful, or zero value and false if empty.
// Only one goroutine should call Read (single consumer).
func (rb *SPSCRingBuffer[T]) Read() (T, bool) {
	read := atomic.LoadUint64(&rb.read)
	write := atomic.LoadUint64(&rb.write)

	// Check if empty
	if read == write {
		var zero T
		return zero, false
	}

	// Read the item
	item := rb.items[read&rb.mask]

	// Memory barrier: ensure item is read before read pointer is updated
	// This guarantees the producer sees the freed slot when it sees the updated read pointer
	atomic.StoreUint64(&rb.read, read+1)
	return item, true
}

// IsEmpty returns true if the ring buffer is empty.
func (rb *SPSCRingBuffer[T]) IsEmpty() bool {
	return atomic.LoadUint64(&rb.read) == atomic.LoadUint64(&rb.write)
}

// IsFull returns true if the ring buffer is full.
func (rb *SPSCRingBuffer[T]) IsFull() bool {
	return rb.Count() >= uint64(len(rb.items))
}

// Count returns the number of items currently in the ring buffer.
func (rb *SPSCRingBuffer[T]) Count() uint64 {
	return atomic.LoadUint64(&rb.write) - atomic.LoadUint64(&rb.read)
}

// Capacity returns the capacity of the ring buffer.
func (rb *SPSCRingBuffer[T]) Capacity() uint64 {
	return uint64(len(rb.items))
}

// Peek returns the oldest item without removing it from the buffer.
// Returns the item and true if successful, or zero value and false if empty.
// Only one goroutine should call Peek (single consumer).
func (rb *SPSCRingBuffer[T]) Peek() (T, bool) {
	read := atomic.LoadUint64(&rb.read)
	write := atomic.LoadUint64(&rb.write)

	// Check if empty
	if read == write {
		var zero T
		return zero, false
	}

	// Return the item without advancing read pointer
	return rb.items[read&rb.mask], true
}

// Reset clears the ring buffer by resetting read and write pointers.
// WARNING: This is NOT thread-safe and should only be called when no operations are in progress.
func (rb *SPSCRingBuffer[T]) Reset() {
	atomic.StoreUint64(&rb.read, 0)
	atomic.StoreUint64(&rb.write, 0)
}

// =============================================================================
// MPSC Ring Buffer - Multiple Producer Single Consumer
// =============================================================================

// MPSCRingBuffer is a lock-free Multiple Producer Single Consumer ring buffer.
// It uses atomic CAS operations for concurrent producers and a two-phase commit protocol.
//
// Performance characteristics:
//   - ~30-50ns per operation under contention
//   - ~15-25ns per operation without contention
//   - Cache-line padding prevents false sharing
//
// Two-phase commit protocol:
//  1. Producer claims a slot by CAS on write pointer
//  2. Producer writes data to the slot
//  3. Producer commits by advancing the committed pointer (in order)
//
// Thread safety:
//   - Multiple goroutines can safely call Write (multiple producers)
//   - Exactly ONE goroutine should call Read (single consumer)
//   - IsEmpty, IsFull, Count, Capacity are safe to call from any goroutine
type MPSCRingBuffer[T any] struct {
	items     []T
	mask      uint64
	_pad0     [56]byte
	write     uint64 // Producers compete for this (CAS)
	_pad1     [56]byte
	committed uint64 // Last committed write position (ensures in-order visibility)
	_pad2     [56]byte
	read      uint64 // Consumer reads here
	_pad3     [56]byte
}

// NewMPSCRingBuffer creates a new MPSC ring buffer with the specified size.
// Size will be rounded up to the next power of 2 for efficient masking.
func NewMPSCRingBuffer[T any](size uint64) *MPSCRingBuffer[T] {
	actualSize := nextPowerOf2(size)
	return &MPSCRingBuffer[T]{
		items:     make([]T, actualSize),
		mask:      actualSize - 1,
		write:     0,
		committed: 0,
		read:      0,
	}
}

// Write writes an item to the ring buffer.
// Returns true if successful, false if the buffer is full.
// Multiple goroutines can safely call Write concurrently.
func (rb *MPSCRingBuffer[T]) Write(item T) bool {
	for {
		write := atomic.LoadUint64(&rb.write)
		read := atomic.LoadUint64(&rb.read)

		// Check if full
		if write-read >= uint64(len(rb.items)) {
			return false
		}

		// Try to claim this slot using CAS
		if atomic.CompareAndSwapUint64(&rb.write, write, write+1) {
			// We claimed the slot at position 'write', now write the item
			rb.items[write&rb.mask] = item

			// Two-phase commit: wait for all previous writers to commit, then commit ours
			// This ensures items become visible to the consumer in order
			for !atomic.CompareAndSwapUint64(&rb.committed, write, write+1) {
				// Spin until previous writers commit
				// This is typically very short as writers commit quickly after writing
			}
			return true
		}
		// CAS failed, another producer claimed the slot, retry with new position
	}
}

// Read reads an item from the ring buffer.
// Returns the item and true if successful, or zero value and false if empty.
// Only one goroutine should call Read (single consumer).
func (rb *MPSCRingBuffer[T]) Read() (T, bool) {
	read := atomic.LoadUint64(&rb.read)
	committed := atomic.LoadUint64(&rb.committed)

	// Check if empty - only read items that have been committed
	if read == committed {
		var zero T
		return zero, false
	}

	// Read the item - it's guaranteed to be fully written since it's committed
	item := rb.items[read&rb.mask]

	// Memory barrier: ensure item is read before read pointer is updated
	atomic.StoreUint64(&rb.read, read+1)
	return item, true
}

// IsEmpty returns true if the ring buffer has no committed items to read.
func (rb *MPSCRingBuffer[T]) IsEmpty() bool {
	return atomic.LoadUint64(&rb.read) == atomic.LoadUint64(&rb.committed)
}

// IsFull returns true if the ring buffer is full.
func (rb *MPSCRingBuffer[T]) IsFull() bool {
	write := atomic.LoadUint64(&rb.write)
	read := atomic.LoadUint64(&rb.read)
	return write-read >= uint64(len(rb.items))
}

// Count returns the number of committed items currently available to read.
func (rb *MPSCRingBuffer[T]) Count() uint64 {
	return atomic.LoadUint64(&rb.committed) - atomic.LoadUint64(&rb.read)
}

// PendingCount returns the number of items claimed but not yet committed.
// This can be useful for debugging or monitoring.
func (rb *MPSCRingBuffer[T]) PendingCount() uint64 {
	return atomic.LoadUint64(&rb.write) - atomic.LoadUint64(&rb.committed)
}

// Capacity returns the capacity of the ring buffer.
func (rb *MPSCRingBuffer[T]) Capacity() uint64 {
	return uint64(len(rb.items))
}

// Reset clears the ring buffer by resetting all pointers.
// WARNING: This is NOT thread-safe and should only be called when no operations are in progress.
func (rb *MPSCRingBuffer[T]) Reset() {
	atomic.StoreUint64(&rb.read, 0)
	atomic.StoreUint64(&rb.write, 0)
	atomic.StoreUint64(&rb.committed, 0)
}
