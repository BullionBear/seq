package ob

const (
	// DefaultPriceLevelBufferSize is the default capacity for price level ring buffers
	DefaultPriceLevelBufferSize = 1000

	// DefaultDepthUpdateBufferSize is the default capacity for depth update ring buffers
	DefaultDepthUpdateBufferSize = 100
)

// PriceLevelRingBuffer is a circular buffer for PriceLevel storage.
// It provides zero-allocation storage for price levels that can be reused.
type PriceLevelRingBuffer struct {
	items    []PriceLevel
	capacity int
	writeIdx int
}

// NewPriceLevelRingBuffer creates a new PriceLevelRingBuffer with the given capacity.
func NewPriceLevelRingBuffer(capacity int) *PriceLevelRingBuffer {
	return &PriceLevelRingBuffer{
		items:    make([]PriceLevel, capacity),
		capacity: capacity,
		writeIdx: 0,
	}
}

// Allocate reserves n consecutive PriceLevel slots and returns a slice to them.
// The returned slice is valid until the buffer wraps around.
func (rb *PriceLevelRingBuffer) Allocate(n int) []PriceLevel {
	if n > rb.capacity {
		// If request exceeds capacity, wrap around and start from beginning
		rb.writeIdx = 0
	}

	// Check if we need to wrap
	if rb.writeIdx+n > rb.capacity {
		rb.writeIdx = 0
	}

	start := rb.writeIdx
	rb.writeIdx += n
	return rb.items[start : start+n]
}

// Reset resets the buffer to initial state
func (rb *PriceLevelRingBuffer) Reset() {
	rb.writeIdx = 0
}

// Capacity returns the buffer capacity
func (rb *PriceLevelRingBuffer) Capacity() int {
	return rb.capacity
}

// DepthUpdateRingBuffer is a circular buffer for buffering DepthUpdates during sync.
// It maintains FIFO order for processing buffered updates.
type DepthUpdateRingBuffer struct {
	items    []DepthUpdateBuffer
	capacity int
	readIdx  int
	writeIdx int
	count    int
}

// NewDepthUpdateRingBuffer creates a new DepthUpdateRingBuffer with the given capacity.
func NewDepthUpdateRingBuffer(capacity int) *DepthUpdateRingBuffer {
	return &DepthUpdateRingBuffer{
		items:    make([]DepthUpdateBuffer, capacity),
		capacity: capacity,
		readIdx:  0,
		writeIdx: 0,
		count:    0,
	}
}

// Push adds a depth update to the buffer.
// Returns false if buffer is full (oldest item will be overwritten).
func (rb *DepthUpdateRingBuffer) Push(update DepthUpdateBuffer) bool {
	wasFull := rb.count >= rb.capacity

	rb.items[rb.writeIdx] = update
	rb.writeIdx = (rb.writeIdx + 1) % rb.capacity

	if wasFull {
		// Overwriting oldest - advance read pointer
		rb.readIdx = (rb.readIdx + 1) % rb.capacity
	} else {
		rb.count++
	}

	return !wasFull
}

// Pop removes and returns the oldest depth update from the buffer.
// Returns nil if buffer is empty.
func (rb *DepthUpdateRingBuffer) Pop() *DepthUpdateBuffer {
	if rb.count == 0 {
		return nil
	}

	item := &rb.items[rb.readIdx]
	rb.readIdx = (rb.readIdx + 1) % rb.capacity
	rb.count--
	return item
}

// Peek returns the oldest depth update without removing it.
// Returns nil if buffer is empty.
func (rb *DepthUpdateRingBuffer) Peek() *DepthUpdateBuffer {
	if rb.count == 0 {
		return nil
	}
	return &rb.items[rb.readIdx]
}

// Count returns the number of items in the buffer.
func (rb *DepthUpdateRingBuffer) Count() int {
	return rb.count
}

// IsEmpty returns true if the buffer is empty.
func (rb *DepthUpdateRingBuffer) IsEmpty() bool {
	return rb.count == 0
}

// IsFull returns true if the buffer is full.
func (rb *DepthUpdateRingBuffer) IsFull() bool {
	return rb.count >= rb.capacity
}

// Reset clears the buffer.
func (rb *DepthUpdateRingBuffer) Reset() {
	rb.readIdx = 0
	rb.writeIdx = 0
	rb.count = 0
}

// Capacity returns the buffer capacity.
func (rb *DepthUpdateRingBuffer) Capacity() int {
	return rb.capacity
}

// Iterate calls the given function for each buffered update in FIFO order.
// The function should return true to continue iteration, false to stop.
func (rb *DepthUpdateRingBuffer) Iterate(fn func(*DepthUpdateBuffer) bool) {
	for i := 0; i < rb.count; i++ {
		idx := (rb.readIdx + i) % rb.capacity
		if !fn(&rb.items[idx]) {
			break
		}
	}
}
