package evbus

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event wraps data with metadata. Data is embedded as a value type
// so Event and Data are pooled together (single allocation).
type Event[T any] struct {
	Data      T // Embedded value, pooled together with Event
	EventID   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EventFactory creates and recycles events (lock-free).
// Uses value types to avoid separate allocations for Data.
type EventFactory[T any] struct {
	nextEventID atomic.Int64
	eventPool   sync.Pool
	resetFn     func(*T) // Reset function for Data cleanup
}

// NewEventFactory creates a new lock-free event factory.
// resetFn is called on Data when returning event to pool.
func NewEventFactory[T any](resetFn func(*T)) *EventFactory[T] {
	return &EventFactory[T]{
		eventPool: sync.Pool{
			New: func() any { return new(Event[T]) },
		},
		resetFn: resetFn,
	}
}

// GetEvent retrieves a pooled event (lock-free).
// Data is zero-valued; set fields directly on event.Data.
func (f *EventFactory[T]) GetEvent() *Event[T] {
	event := f.eventPool.Get().(*Event[T])
	event.EventID = f.nextEventID.Add(1)
	now := time.Now().UTC()
	event.CreatedAt = now
	event.UpdatedAt = now
	return event
}

// PutEvent returns event to pool (lock-free).
// Calls resetFn to clean up Data before pooling.
func (f *EventFactory[T]) PutEvent(event *Event[T]) {
	if f.resetFn != nil {
		f.resetFn(&event.Data)
	}
	event.EventID = 0
	event.CreatedAt = time.Time{}
	event.UpdatedAt = time.Time{}
	f.eventPool.Put(event)
}
