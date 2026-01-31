package ob

const (
	// DefaultPriceLevelBufferSize is the default capacity for price level arena
	DefaultPriceLevelBufferSize = 1000

	// DefaultDepthUpdateBufferSize is the default capacity for depth update ring buffers
	DefaultDepthUpdateBufferSize = 100
)

// PriceLevelArena is a circular arena for PriceLevel storage.
// It provides zero-allocation storage for price levels that can be reused.
// This is NOT a FIFO queue - it allocates contiguous slices that wrap around.
//
// Note: This is single-threaded (not thread-safe).
type PriceLevelArena struct {
	items    []PriceLevel
	capacity int
	writeIdx int
}

// NewPriceLevelArena creates a new PriceLevelArena with the given capacity.
func NewPriceLevelArena(capacity int) *PriceLevelArena {
	return &PriceLevelArena{
		items:    make([]PriceLevel, capacity),
		capacity: capacity,
		writeIdx: 0,
	}
}

// Allocate reserves n consecutive PriceLevel slots and returns a slice to them.
// The returned slice is valid until the buffer wraps around.
func (a *PriceLevelArena) Allocate(n int) []PriceLevel {
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
func (a *PriceLevelArena) Reset() {
	a.writeIdx = 0
}

// Capacity returns the arena capacity
func (a *PriceLevelArena) Capacity() int {
	return a.capacity
}
