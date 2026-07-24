package msgbus

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
)

const (
	// DefaultByteArenaCapacity is the default capacity for the byte arena (1MB)
	DefaultByteArenaCapacity = 1024 * 1024

	// DefaultEventRingBufferSize is the default size for the event ring buffer
	DefaultEventRingBufferSize = 4096

	// DefaultOverflowDeadline is how long a critical-class producer waits for
	// ring/arena space before the process fails hard. Overflow of critical
	// topics means the dispatch loop has been stalled for this long — the
	// system can no longer guarantee order/balance state consistency.
	DefaultOverflowDeadline = 5 * time.Second

	// droppableSpinBudget is the number of Gosched-yielding retries a
	// droppable-class producer performs before dropping the event.
	droppableSpinBudget = 64
)

// EventHandler is a function type for handling events
type EventHandler func(event Event)

// EventBus is the central event distribution system.
// It uses a lock-free MPSC (Multiple Producer Single Consumer) ring buffer
// for events, allowing multiple goroutines to publish events concurrently
// while a single consumer (the dispatch loop) processes them.
//
// Overflow policy (per topic class, see event.Topic.IsDroppable):
//   - Critical topics (engine state, order lifecycle, execution, balance):
//     Allocate/Publish spin (with runtime.Gosched escalation) until space
//     frees; after the configured overflow deadline the process fails hard
//     with a fatal log. Critical events are NEVER dropped.
//   - Droppable topics (depth, tick, timer): after a bounded spin the event
//     is dropped, the arena reservation is released, and the per-topic drop
//     counter is incremented. Consumers recover via snapshot re-request
//     (orderbook DepthID gap detection) or the next timer tick.
//
// Deadlock note: a publisher running ON the dispatch goroutine (command
// processors, notifier calls from handlers) cannot be drained past a full
// ring, since it is itself the consumer. The overflow deadline converts that
// would-be deadlock into a detected fatal. Size the ring so that
// dispatch-thread publishers always have headroom.
//
// Thread safety:
//   - Allocate/Publish/Cancel can be called from multiple goroutines
//   - Dispatch should be called from a single goroutine
type EventBus struct {
	nextEventID uint64

	// MPSC ring buffer for events - supports concurrent producers
	rbEvent *mem.MPSCRingBuffer[Event]

	// Single byte arena for all event data (cache-friendly)
	byteArena *mem.CircularByteArena

	// Multi-consumer support
	consumers   []*Consumer
	minSequence uint64 // minimum sequence across all consumers (for arena release)

	// Optional logger for persisting events as JSONL
	msgLogger *MsgLogger

	// Overflow accounting (indexed by topic).
	dropCounts [event.TopicCount]uint64
	waitCounts [event.TopicCount]uint64

	overflowDeadline time.Duration
}

// NewEventBus creates a new EventBus with default capacity.
func NewEventBus() *EventBus {
	return NewEventBusWithCapacity(DefaultByteArenaCapacity)
}

// NewEventBusWithCapacity creates a new EventBus with the specified byte arena capacity.
func NewEventBusWithCapacity(arenaCapacity uint64) *EventBus {
	return &EventBus{
		nextEventID:      0,
		rbEvent:          mem.NewMPSCRingBuffer[Event](DefaultEventRingBufferSize),
		byteArena:        mem.NewCircularByteArena(arenaCapacity),
		consumers:        make([]*Consumer, 0),
		minSequence:      0,
		overflowDeadline: DefaultOverflowDeadline,
	}
}

// SetMsgLogger sets the message logger for persisting events as JSONL.
func (e *EventBus) SetMsgLogger(l *MsgLogger) {
	e.msgLogger = l
}

// SetOverflowDeadline configures how long critical-class producers wait for
// space before failing hard. Must be called before concurrent publishing.
func (e *EventBus) SetOverflowDeadline(d time.Duration) {
	e.overflowDeadline = d
}

// DropCount returns the number of events dropped for the given topic.
// Only droppable-class topics can ever have a non-zero count.
func (e *EventBus) DropCount(topic event.Topic) uint64 {
	return atomic.LoadUint64(&e.dropCounts[topic])
}

// WaitCount returns the number of Allocate/Publish calls for the given topic
// that had to wait for ring or arena space.
func (e *EventBus) WaitCount(topic event.Topic) uint64 {
	return atomic.LoadUint64(&e.waitCounts[topic])
}

// RegisterPhased adds a consumer at the given dispatch phase.
// If topics is nil or empty, the consumer receives all topics.
// Phases must be non-decreasing across registrations; AssertOrder enforces it.
// See docs/CONSUMER_ORDER.md.
func (e *EventBus) RegisterPhased(phase Phase, name string, topics []event.Topic, handler EventHandler) {
	consumer := NewConsumer(name, topics, handler)
	consumer.Phase = phase
	e.consumers = append(e.consumers, consumer)
}

// ConsumerNames returns registered consumer names in dispatch order.
// Allocates; startup and test paths only, never called from Dispatch.
func (e *EventBus) ConsumerNames() []string {
	names := make([]string, len(e.consumers))
	for i, c := range e.consumers {
		names[i] = c.Name
	}
	return names
}

// ConsumerPhases returns registered consumer phases in dispatch order.
// Allocates; startup and test paths only, never called from Dispatch.
func (e *EventBus) ConsumerPhases() []Phase {
	phases := make([]Phase, len(e.consumers))
	for i, c := range e.consumers {
		phases[i] = c.Phase
	}
	return phases
}

// AssertOrder verifies consumer phases are non-decreasing, i.e. that every
// cache writer is registered before every cache reader.
// Called once from Node.initEngines after all engines are initialized.
// See docs/CONSUMER_ORDER.md.
func (e *EventBus) AssertOrder() error {
	prev := Phase(-1)
	prevName := "<start>"
	for _, c := range e.consumers {
		if c.Phase < prev {
			return fmt.Errorf(
				"consumer order violation: %q (phase %s) registered after %q (phase %s); "+
					"cache writers must precede readers, see docs/CONSUMER_ORDER.md",
				c.Name, c.Phase, prevName, prev)
		}
		prev, prevName = c.Phase, c.Name
	}
	return nil
}

// Dispatch reads the next event from the ring buffer and dispatches it to all
// consumers whose topic filter matches. Returns true if an event was dispatched,
// false if the ring buffer is empty.
// After all consumers have processed the event, its arena reservation is
// released — event payload slices must not be retained past the handler.
func (e *EventBus) Dispatch() bool {
	ev, ok := e.rbEvent.Read()
	if !ok {
		return false
	}

	if e.msgLogger != nil {
		payload := e.byteArena.ReadSlice(ev.Ref.Index, ev.Ref.Length)
		e.msgLogger.LogEvent(ev, payload)
	}

	// Dispatch to all consumers whose topic filter matches
	for _, consumer := range e.consumers {
		if consumer.ShouldHandle(ev.Ref.Topic) {
			consumer.Handler(ev)
		}
		// Update consumer sequence regardless of whether it handled the event
		// This ensures minSequence advances correctly
		consumer.Sequence = ev.EventID
	}

	e.byteArena.Release(ev.Ref.reservation())
	return true
}

// Release updates minSequence to the minimum sequence across all consumers.
// This indicates the oldest event that is still being processed.
func (e *EventBus) Release() {
	if len(e.consumers) == 0 {
		return
	}

	min := e.consumers[0].Sequence
	for _, consumer := range e.consumers[1:] {
		if consumer.Sequence < min {
			min = consumer.Sequence
		}
	}
	e.minSequence = min
}

// MinSequence returns the minimum sequence across all consumers.
func (e *EventBus) MinSequence() uint64 {
	return e.minSequence
}

// ReleaseArenas is kept for backward compatibility. Arena space is released
// per event by Dispatch/Poll.
func (e *EventBus) ReleaseArenas() {
	// No-op: reservations are released as events are dispatched.
}

// ConsumerCount returns the number of registered consumers.
func (e *EventBus) ConsumerCount() int {
	return len(e.consumers)
}

// Poll reads the next event from the ring buffer and calls the handler.
// The event's arena reservation is released after the handler returns.
func (e *EventBus) Poll(handler EventHandler) bool {
	ev, ok := e.rbEvent.Read()
	if !ok {
		return false
	}
	if e.msgLogger != nil {
		payload := e.byteArena.ReadSlice(ev.Ref.Index, ev.Ref.Length)
		e.msgLogger.LogEvent(ev, payload)
	}
	handler(ev)
	e.byteArena.Release(ev.Ref.reservation())
	return true
}

// Allocate reserves arena space for an event of the given topic and returns a
// fully-populated EventRef plus the []byte slice to serialize into. After
// encoding, pass the EventRef to Publish (or Cancel if publishing is
// abandoned — the reservation must not leak).
//
// Returns ok=false only for droppable topics when the arena stays full past
// the spin budget (the drop counter is incremented). Critical topics never
// fail: they wait, and fail hard after the overflow deadline.
func (e *EventBus) Allocate(topic event.Topic, size uint64) (EventRef, []byte, bool) {
	res, ok := e.byteArena.TryReserve(size)
	if !ok {
		res, ok = e.reserveSlow(topic, size)
		if !ok {
			return EventRef{}, nil, false
		}
	}
	ref := EventRef{
		Topic:    topic,
		Index:    res.Offset,
		Length:   size,
		resStart: res.Start,
		resEnd:   res.End,
	}
	return ref, e.byteArena.GetSlice(res.Offset, size), true
}

// reserveSlow is the overflow path of Allocate.
func (e *EventBus) reserveSlow(topic event.Topic, size uint64) (mem.Reservation, bool) {
	atomic.AddUint64(&e.waitCounts[topic], 1)
	droppable := topic.IsDroppable()
	var deadline time.Time
	spins := 0
	for {
		runtime.Gosched()
		if res, ok := e.byteArena.TryReserve(size); ok {
			return res, true
		}
		if droppable {
			spins++
			if spins >= droppableSpinBudget {
				atomic.AddUint64(&e.dropCounts[topic], 1)
				return mem.Reservation{}, false
			}
		} else {
			if deadline.IsZero() {
				deadline = time.Now().Add(e.overflowDeadline)
			} else if time.Now().After(deadline) {
				log().Fatal().
					Str("topic", topic.String()).
					Uint64("size", size).
					Dur("deadline", e.overflowDeadline).
					Msg("EventBus: arena full past overflow deadline for critical topic; dispatch loop stalled")
			}
		}
	}
}

// Publish publishes an EventRef to the ring buffer. The caller must have
// serialized data into the arena buffer obtained via Allocate.
// This method is thread-safe and can be called from multiple goroutines.
//
// Returns false only for droppable topics when the ring stays full past the
// spin budget; the event is dropped, its arena reservation released, and the
// drop counter incremented. Critical topics never fail (see EventBus doc).
func (e *EventBus) Publish(ref EventRef) bool {
	now := uint64(time.Now().UnixNano())
	// Use atomic increment to get unique event ID for concurrent publishers
	eventID := atomic.AddUint64(&e.nextEventID, 1) - 1
	ev := Event{
		Ref:       ref,
		EventID:   eventID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if e.rbEvent.Write(ev) {
		return true
	}
	return e.publishSlow(ev)
}

// publishSlow is the overflow path of Publish.
func (e *EventBus) publishSlow(ev Event) bool {
	topic := ev.Ref.Topic
	atomic.AddUint64(&e.waitCounts[topic], 1)
	droppable := topic.IsDroppable()
	var deadline time.Time
	spins := 0
	for {
		runtime.Gosched()
		if e.rbEvent.Write(ev) {
			return true
		}
		if droppable {
			spins++
			if spins >= droppableSpinBudget {
				e.byteArena.Release(ev.Ref.reservation())
				atomic.AddUint64(&e.dropCounts[topic], 1)
				return false
			}
		} else {
			if deadline.IsZero() {
				deadline = time.Now().Add(e.overflowDeadline)
			} else if time.Now().After(deadline) {
				log().Fatal().
					Str("topic", topic.String()).
					Uint64("eventID", ev.EventID).
					Dur("deadline", e.overflowDeadline).
					Msg("EventBus: ring full past overflow deadline for critical topic; dispatch loop stalled")
			}
		}
	}
}

// Cancel releases the arena reservation of an allocated-but-never-published
// EventRef (e.g. after an encode error). Every Allocate must be balanced by
// exactly one Publish or Cancel.
func (e *EventBus) Cancel(ref EventRef) {
	e.byteArena.Release(ref.reservation())
}

// ReadBuffer returns a []byte slice at the given offset/length for consumers
// to deserialize event data from. The slice is valid only until the event's
// reservation is released (i.e. within the dispatch handler).
func (e *EventBus) ReadBuffer(offset, length uint64) []byte {
	return e.byteArena.ReadSlice(offset, length)
}
