package msgbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/model/event"
)

// TestEventBus_CriticalNoLossUnderFullRing is the P0-1 verification for the
// critical class: multiple producers publish far more critical events than
// the ring can hold while the dispatcher is deliberately paused for 100ms
// (chaos), then drains. Every single event must arrive; drop counters must
// stay zero.
func TestEventBus_CriticalNoLossUnderFullRing(t *testing.T) {
	bus := NewEventBus()
	bus.SetOverflowDeadline(30 * time.Second) // never hit in a healthy run

	const numProducers = 4
	const perProducer = 5000
	const total = numProducers * perProducer

	var received int64
	seen := make(map[int]bool, total)
	var seenMu sync.Mutex
	bus.Register("counter", []event.Topic{event.TopicEventOrderNew}, func(ev Event) {
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderNew, err := event.NewOrderNewFromBytes(buf)
		if err != nil {
			t.Errorf("decode failed: %v", err)
			return
		}
		seenMu.Lock()
		if seen[orderNew.ClientOrderID] {
			t.Errorf("duplicate event %d", orderNew.ClientOrderID)
		}
		seen[orderNew.ClientOrderID] = true
		seenMu.Unlock()
		atomic.AddInt64(&received, 1)
	})

	var wg sync.WaitGroup
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				ev := event.OrderNew{ClientOrderID: id*perProducer + i, AccountID: id}
				ref, buf, ok := bus.Allocate(event.TopicEventOrderNew, uint64(ev.GetBufferLength()))
				if !ok {
					t.Errorf("critical Allocate returned ok=false")
					return
				}
				if err := ev.Encode(buf); err != nil {
					t.Errorf("encode failed: %v", err)
					bus.Cancel(ref)
					return
				}
				if !bus.Publish(ref) {
					t.Errorf("critical Publish returned false")
					return
				}
			}
		}(p)
	}

	// Chaos: dispatcher pauses 100ms while producers hammer a full ring.
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	deadline := time.After(30 * time.Second)
	producersDone := false
	for {
		worked := bus.Dispatch()
		if !worked {
			if producersDone && atomic.LoadInt64(&received) == total {
				break
			}
			select {
			case <-done:
				producersDone = true
			case <-deadline:
				t.Fatalf("timeout: received %d of %d events", atomic.LoadInt64(&received), total)
			default:
			}
		}
	}

	if got := atomic.LoadInt64(&received); got != total {
		t.Errorf("expected %d events, got %d — critical events were lost", total, got)
	}
	if drops := bus.DropCount(event.TopicEventOrderNew); drops != 0 {
		t.Errorf("critical topic has non-zero drop count %d", drops)
	}
}

// TestEventBus_DroppableAccountedUnderFullRing is the P0-1 verification for
// the droppable class: with no dispatcher draining, droppable publishes must
// eventually fail (never block forever), every drop must be counted, and no
// arena space may leak (dropped reservations are released).
func TestEventBus_DroppableAccountedUnderFullRing(t *testing.T) {
	bus := NewEventBus()
	bus.SetOverflowDeadline(30 * time.Second)

	var received int64
	bus.Register("counter", []event.Topic{event.TopicEventTick}, func(ev Event) {
		atomic.AddInt64(&received, 1)
	})

	const total = 20000 // ring holds 4096; most of these must be dropped
	published := int64(0)
	tick := event.Tick{SymbolID: 1, Price: 42}
	size := uint64(tick.GetBufferLength())

	for i := 0; i < total; i++ {
		ref, buf, ok := bus.Allocate(event.TopicEventTick, size)
		if !ok {
			continue // dropped at allocation, counted by the bus
		}
		if err := tick.Encode(buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		if bus.Publish(ref) {
			published++
		}
	}

	drops := bus.DropCount(event.TopicEventTick)
	if published+int64(drops) != total {
		t.Errorf("accounting mismatch: published %d + dropped %d != total %d", published, drops, total)
	}
	if drops == 0 {
		t.Error("expected drops with no dispatcher draining, got none")
	}

	// Drain: everything successfully published must arrive.
	for bus.Dispatch() {
	}
	if got := atomic.LoadInt64(&received); got != published {
		t.Errorf("expected %d dispatched events, got %d", published, got)
	}

	// No arena leak: dropped reservations were released, so after draining,
	// claimed == released.
	// One more allocate/publish/dispatch round must work.
	ref, buf, ok := bus.Allocate(event.TopicEventTick, size)
	if !ok {
		t.Fatal("arena leaked: allocation fails after full drain")
	}
	if err := tick.Encode(buf); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if !bus.Publish(ref) {
		t.Fatal("publish failed after full drain")
	}
	if !bus.Dispatch() {
		t.Fatal("dispatch failed after full drain")
	}
}

// TestEventBus_ArenaReleasedPerEvent verifies the P0-2 wiring: arena space is
// returned as events are dispatched, so a stream much larger than the arena
// capacity flows without stalling.
func TestEventBus_ArenaReleasedPerEvent(t *testing.T) {
	bus := NewEventBusWithCapacity(4096) // deliberately tiny arena
	bus.SetOverflowDeadline(30 * time.Second)

	var received int64
	bus.Register("counter", nil, func(ev Event) {
		atomic.AddInt64(&received, 1)
	})

	const total = 10000
	ev := event.OrderNew{AccountID: 7}
	size := uint64(ev.GetBufferLength())

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			ref, buf, ok := bus.Allocate(event.TopicEventOrderNew, size)
			if !ok {
				t.Errorf("critical Allocate returned ok=false")
				return
			}
			if err := ev.Encode(buf); err != nil {
				t.Errorf("encode failed: %v", err)
				bus.Cancel(ref)
				return
			}
			bus.Publish(ref)
		}
	}()

	deadline := time.After(30 * time.Second)
	for atomic.LoadInt64(&received) < total {
		if !bus.Dispatch() {
			select {
			case <-deadline:
				t.Fatalf("timeout: received %d of %d", atomic.LoadInt64(&received), total)
			default:
			}
		}
	}
	<-done
}
