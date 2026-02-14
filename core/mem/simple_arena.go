package mem

// SimpleByteArena is a non-atomic circular byte arena for single-threaded use.
// It provides the same API as CircularByteArena but without CAS operations,
// making it suitable for SPSC (Single Producer Single Consumer) scenarios
// where both producer and consumer run on the same goroutine.
//
// Thread safety:
//   - NOT thread-safe. Must be accessed from a single goroutine only.
//
// Usage pattern:
//  1. Call Reserve(size) to claim space
//  2. Call GetSlice(offset, size) to get a writable buffer
//  3. Write data to the buffer
//  4. Consumer calls ReadSlice(offset, size) to read data
type SimpleByteArena struct {
	buffer   []byte
	capacity uint64
	writeOff uint64
}

// NewSimpleByteArena creates a new SimpleByteArena with the specified capacity.
func NewSimpleByteArena(capacity uint64) *SimpleByteArena {
	return &SimpleByteArena{
		buffer:   make([]byte, capacity),
		capacity: capacity,
		writeOff: 0,
	}
}

// Reserve reserves space in the buffer and returns the offset where data should be written.
// Handles wrap-around: if writeOff + size > capacity, wraps to beginning.
//
// Note: Allocations larger than capacity will cause issues - caller must ensure size <= capacity.
func (a *SimpleByteArena) Reserve(size uint64) uint64 {
	offset := a.writeOff
	newOff := offset + size

	// Check for wrap-around: if not enough space at the end, wrap to beginning
	if newOff > a.capacity {
		offset = 0
		newOff = size
	}

	a.writeOff = newOff
	return offset
}

// GetSlice returns a writable slice at the given offset for producers to write data into.
// This should be called after Reserve() to get the buffer slice for serialization.
func (a *SimpleByteArena) GetSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// ReadSlice returns a slice into the buffer at the given offset with the specified size.
// The returned slice points directly into the buffer - no copy is made.
func (a *SimpleByteArena) ReadSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// WriteOff returns the current write offset.
func (a *SimpleByteArena) WriteOff() uint64 {
	return a.writeOff
}

// Capacity returns the buffer capacity.
func (a *SimpleByteArena) Capacity() uint64 {
	return a.capacity
}

// Reset resets the arena to its initial state.
func (a *SimpleByteArena) Reset() {
	a.writeOff = 0
}
