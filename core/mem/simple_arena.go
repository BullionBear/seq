package mem

// SimpleByteArena is a bounded, non-atomic circular byte arena for
// single-threaded use (the dispatch goroutine produces and consumes commands
// on the same thread). It provides the same reservation/release contract as
// CircularByteArena but without CAS operations.
//
// Visibility / overwrite contract: TryReserve never hands out a range that
// overlaps an unreleased range, so unconsumed data can never be overwritten.
// Because producer and consumer share one goroutine, a full arena cannot be
// waited out — callers must treat ok=false as a fatal configuration error.
//
// Thread safety: NOT thread-safe. Must be accessed from a single goroutine.
type SimpleByteArena struct {
	buffer   []byte
	capacity uint64
	claimed  uint64 // monotonic
	released uint64 // monotonic frontier
	pending  []pendingRange
}

// NewSimpleByteArena creates a new SimpleByteArena with the specified
// capacity, rounded up to a multiple of 8 for aligned offsets.
func NewSimpleByteArena(capacity uint64) *SimpleByteArena {
	capacity = (capacity + alignment - 1) &^ (alignment - 1)
	return &SimpleByteArena{
		buffer:   make([]byte, capacity),
		capacity: capacity,
		pending:  make([]pendingRange, 0, 8),
	}
}

// TryReserve attempts to claim size bytes (rounded up to 8-byte alignment).
// The payload never straddles the physical end of the buffer. Returns
// ok=false when there is not enough unreleased space; size must be
// <= Capacity().
func (a *SimpleByteArena) TryReserve(size uint64) (Reservation, bool) {
	size = (size + alignment - 1) &^ (alignment - 1)
	if size > a.capacity {
		panic("mem: SimpleByteArena reservation exceeds capacity")
	}
	phys := a.claimed % a.capacity
	var pad uint64
	if phys+size > a.capacity {
		pad = a.capacity - phys
	}
	total := pad + size
	if a.claimed+total-a.released > a.capacity {
		return Reservation{}, false
	}
	res := Reservation{
		Start:  a.claimed,
		End:    a.claimed + total,
		Offset: (a.claimed + pad) % a.capacity,
	}
	a.claimed += total
	return res, true
}

// Release returns a claimed range to the arena. Commands are normally
// released in reservation order, but out-of-order releases are tolerated.
func (a *SimpleByteArena) Release(r Reservation) {
	if r.End == r.Start {
		return
	}
	if r.Start == a.released {
		a.released = r.End
		for {
			merged := false
			for i := 0; i < len(a.pending); i++ {
				if a.pending[i].start == a.released {
					a.released = a.pending[i].end
					a.pending[i] = a.pending[len(a.pending)-1]
					a.pending = a.pending[:len(a.pending)-1]
					merged = true
					break
				}
			}
			if !merged {
				break
			}
		}
	} else {
		a.pending = append(a.pending, pendingRange{start: r.Start, end: r.End})
	}
}

// GetSlice returns a writable slice at the given physical offset for the
// owner of the covering Reservation.
func (a *SimpleByteArena) GetSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// ReadSlice returns a read-only view of the buffer at the given physical
// offset. The view is valid only while the covering Reservation is unreleased.
func (a *SimpleByteArena) ReadSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// Claimed returns the monotonic claimed cursor.
func (a *SimpleByteArena) Claimed() uint64 {
	return a.claimed
}

// Released returns the monotonic released frontier.
func (a *SimpleByteArena) Released() uint64 {
	return a.released
}

// Capacity returns the buffer capacity.
func (a *SimpleByteArena) Capacity() uint64 {
	return a.capacity
}

// Reset resets the arena to its initial state.
func (a *SimpleByteArena) Reset() {
	a.claimed = 0
	a.released = 0
	a.pending = a.pending[:0]
}
