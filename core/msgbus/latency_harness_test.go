package msgbus

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/model/event"
)

// TestPublishDispatchLatencyHarness measures publish -> dispatch handoff
// latency (P2-4 verification): the time from Event.CreatedAt (stamped inside
// Publish, before the ring write) until the consumer handler runs on the
// pinned dispatch goroutine.
//
// It runs closed-loop (the producer waits for each event to be consumed
// before publishing the next) so queueing delay is excluded and the number
// reported is pure handoff plus dispatch overhead. p50/p99/p99.9/max are
// logged.
//
// Default sample count is small enough for CI; for the acceptance run
// (>= 1e8 events on an isolated core, see docs/DEPLOYMENT.md) override with:
//
//	SEQ_LATENCY_EVENTS=100000000 go test ./core/msgbus/ -run LatencyHarness -v -count=1
func TestPublishDispatchLatencyHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("latency harness skipped in -short mode")
	}

	n := 200_000
	if s := os.Getenv("SEQ_LATENCY_EVENTS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			t.Fatalf("invalid SEQ_LATENCY_EVENTS=%q", s)
		}
		n = v
	}

	bus := NewMsgBus()
	latencies := make([]int64, 0, n)
	var consumed atomic.Int64

	bus.Register("latency-harness", nil, func(ev Event) {
		latencies = append(latencies, time.Now().UnixNano()-int64(ev.CreatedAt))
		consumed.Add(1)
	})

	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		// Mirror the production dispatch loop: OS-thread pinned, spin idle
		// strategy (this is the configuration the acceptance criteria target).
		defer close(exited)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for {
			select {
			case <-done:
				return
			default:
				if bus.Dispatch() {
					bus.Release()
				}
			}
		}
	}()

	tick := event.Tick{SymbolID: 1, Timestamp: 1, Price: 1, Qty: 1}
	size := uint64(tick.GetBufferLength())

	for i := 0; i < n; i++ {
		ref, buf, ok := bus.Allocate(event.TopicEventTick, size)
		if !ok {
			t.Fatalf("allocate failed at event %d (closed loop should never overflow)", i)
		}
		if err := tick.Encode(buf); err != nil {
			t.Fatalf("encode: %v", err)
		}
		if !bus.Publish(ref) {
			t.Fatalf("publish failed at event %d", i)
		}
		for consumed.Load() <= int64(i) {
			// Busy-wait: keep the producer hot so the measurement isn't
			// dominated by producer wake-up.
		}
	}
	close(done)
	<-exited

	if len(latencies) != n {
		t.Fatalf("consumed %d of %d events", len(latencies), n)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(q float64) time.Duration {
		idx := int(q * float64(len(latencies)-1))
		return time.Duration(latencies[idx])
	}
	t.Logf("publish->dispatch handoff over %d events: p50=%v p99=%v p99.9=%v max=%v",
		n, pct(0.50), pct(0.99), pct(0.999), time.Duration(latencies[len(latencies)-1]))
}
