package mem

import (
	"runtime"
	"sync/atomic"
)

// alignment is the byte alignment guaranteed for every reservation returned
// by TryReserve. Decoders may rely on 8-byte-aligned payload offsets.
const alignment = 8

// Reservation identifies a claimed byte range in a CircularByteArena.
// Start/End are monotonic (never-wrapping) positions; Offset is the physical
// position of the payload inside the buffer. A Reservation must eventually be
// passed to Release exactly once, otherwise the arena stalls.
type Reservation struct {
	Start  uint64 // monotonic start of the claimed range (includes boundary padding)
	End    uint64 // monotonic end of the claimed range
	Offset uint64 // physical offset of the payload within the buffer
}

// pendingRange is a released range whose start is ahead of the released
// frontier (out-of-order release).
type pendingRange struct {
	start uint64
	end   uint64
}

// CircularByteArena is a bounded circular buffer for storing variable-sized
// event payloads. Multiple producers claim contiguous ranges via TryReserve
// (lock-free CAS); ranges are returned to the arena via Release.
//
// Visibility / overwrite contract:
//   - TryReserve NEVER hands out a range that overlaps a claimed-but-not-yet
//     released range. A producer that cannot make this guarantee receives
//     ok=false and must retry (or drop). Overwriting unread data is therefore
//     impossible by construction — there is no "best effort" mode.
//   - A payload written into GetSlice(res.Offset, size) is immutable from the
//     producer's perspective once published, and remains valid for readers
//     until Release(res) is called. Readers must not retain slices past the
//     release point.
//
// Thread safety:
//   - TryReserve and GetSlice may be called from multiple goroutines.
//   - ReadSlice may be called by any goroutine that holds an unreleased
//     Reservation covering the range.
//   - Release may be called from multiple goroutines (consumer release on the
//     dispatch thread, producer release on the drop path).
type CircularByteArena struct {
	buffer   []byte
	capacity uint64
	_pad0    [48]byte // padding to prevent false sharing
	claimed  uint64   // monotonic; producers compete via CAS
	_pad1    [56]byte
	released uint64 // monotonic frontier; everything below is reusable
	_pad2    [56]byte

	// Out-of-order release bookkeeping, guarded by releaseLock.
	releaseLock uint32
	pending     []pendingRange
}

// NewCircularByteArena creates a new CircularByteArena with the specified
// capacity. The capacity is rounded up to a multiple of 8 so that all
// reservation offsets stay 8-byte aligned across wraps.
func NewCircularByteArena(capacity uint64) *CircularByteArena {
	capacity = (capacity + alignment - 1) &^ (alignment - 1)
	return &CircularByteArena{
		buffer:  make([]byte, capacity),
		pending: make([]pendingRange, 0, 64),

		capacity: capacity,
	}
}

// TryReserve attempts to atomically claim size bytes (rounded up to 8-byte
// alignment). The payload never straddles the physical end of the buffer: if
// it would, the tail is claimed as padding and the payload starts at offset 0.
//
// Returns ok=false when the arena does not currently have room, i.e. the
// consumer has not yet released enough space. The caller decides the wait /
// drop policy. size must be <= Capacity().
func (a *CircularByteArena) TryReserve(size uint64) (Reservation, bool) {
	size = (size + alignment - 1) &^ (alignment - 1)
	if size > a.capacity {
		// Caller bug: allocation can never succeed.
		panic("mem: CircularByteArena reservation exceeds capacity")
	}
	for {
		claimed := atomic.LoadUint64(&a.claimed)
		released := atomic.LoadUint64(&a.released)

		phys := claimed % a.capacity
		var pad uint64
		if phys+size > a.capacity {
			pad = a.capacity - phys
		}
		total := pad + size

		if claimed+total-released > a.capacity {
			return Reservation{}, false
		}
		if atomic.CompareAndSwapUint64(&a.claimed, claimed, claimed+total) {
			return Reservation{
				Start:  claimed,
				End:    claimed + total,
				Offset: (claimed + pad) % a.capacity,
			}, true
		}
		// CAS failed: another producer claimed the range, retry.
	}
}

// Release returns a claimed range to the arena. Releases may arrive out of
// reservation order (producers reserve and publish independently); ranges
// ahead of the frontier are parked until the gap closes.
func (a *CircularByteArena) Release(r Reservation) {
	if r.End == r.Start {
		return
	}
	for !atomic.CompareAndSwapUint32(&a.releaseLock, 0, 1) {
		runtime.Gosched()
	}
	released := atomic.LoadUint64(&a.released)
	if r.Start == released {
		released = r.End
		// Merge any parked ranges that are now contiguous with the frontier.
		for {
			merged := false
			for i := 0; i < len(a.pending); i++ {
				if a.pending[i].start == released {
					released = a.pending[i].end
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
		atomic.StoreUint64(&a.released, released)
	} else {
		a.pending = append(a.pending, pendingRange{start: r.Start, end: r.End})
	}
	atomic.StoreUint32(&a.releaseLock, 0)
}

// GetSlice returns a writable slice at the given physical offset for the
// producer that owns the covering Reservation.
func (a *CircularByteArena) GetSlice(offset, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// ReadSlice returns a read-only view of the buffer at the given physical
// offset. The view is valid only while the covering Reservation is unreleased.
func (a *CircularByteArena) ReadSlice(offset uint64, size uint64) []byte {
	return a.buffer[offset : offset+size]
}

// Claimed returns the monotonic claimed cursor.
func (a *CircularByteArena) Claimed() uint64 {
	return atomic.LoadUint64(&a.claimed)
}

// Released returns the monotonic released frontier.
func (a *CircularByteArena) Released() uint64 {
	return atomic.LoadUint64(&a.released)
}

// Capacity returns the buffer capacity.
func (a *CircularByteArena) Capacity() uint64 {
	return a.capacity
}

// Reset resets the arena to its initial state.
// WARNING: This is NOT thread-safe and should only be called when no
// operations are in progress.
func (a *CircularByteArena) Reset() {
	atomic.StoreUint64(&a.claimed, 0)
	atomic.StoreUint64(&a.released, 0)
	a.pending = a.pending[:0]
}
