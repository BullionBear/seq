package mem

import (
	"runtime"
	"sync/atomic"
)

// commitSpinLimit is how many times a writer busy-spins waiting for an
// earlier producer to commit before yielding the OS thread. Commits are
// normally fast, so a short spin avoids the cost of a scheduler round-trip
// in the common case; yielding afterward keeps a stalled predecessor from
// being starved of CPU time by goroutines that only have work once it
// finishes.
const commitSpinLimit = 64

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
			spins := 0
			for !atomic.CompareAndSwapUint64(&rb.committed, write, write+1) {
				spins++
				if spins > commitSpinLimit {
					// A predecessor hasn't committed yet after a short spin;
					// yield so it (or whatever it's waiting on) gets scheduled
					// instead of losing the core to us spinning.
					runtime.Gosched()
				}
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

// =============================================================================
// Slice Arena - Circular Arena for Contiguous Slice Allocation
// =============================================================================

// SliceArena is a circular arena for allocating contiguous slices of type T.
// It provides zero-allocation storage that can be reused by wrapping around.
//
// This is NOT a FIFO queue - it allocates contiguous slices that wrap around
// when there's not enough space at the end.
//
// Use case: Pre-allocate a pool of objects and get slices from it without
// heap allocations. The returned slices are valid until the arena wraps around.
//
// Note: This is single-threaded (not thread-safe).
type SliceArena[T any] struct {
	items    []T
	capacity int
	writeIdx int
}

// NewSliceArena creates a new SliceArena with the given capacity.
func NewSliceArena[T any](capacity int) *SliceArena[T] {
	return &SliceArena[T]{
		items:    make([]T, capacity),
		capacity: capacity,
		writeIdx: 0,
	}
}

// Allocate reserves n consecutive slots and returns a slice to them.
// The returned slice is valid until the arena wraps around.
// If n > capacity, the arena wraps to the beginning.
func (a *SliceArena[T]) Allocate(n int) []T {
	if n > a.capacity {
		// If request exceeds capacity, wrap around and start from beginning
		a.writeIdx = 0
	}

	// Check if we need to wrap
	if a.writeIdx+n > a.capacity {
		a.writeIdx = 0
	}

	start := a.writeIdx
	a.writeIdx += n
	return a.items[start : start+n]
}

// Reset resets the arena to initial state
func (a *SliceArena[T]) Reset() {
	a.writeIdx = 0
}

// Capacity returns the arena capacity
func (a *SliceArena[T]) Capacity() int {
	return a.capacity
}
