package msgbus

import (
	"context"
	"time"

	"github.com/BullionBear/seq/core/model/event"
)

// DefaultObserverInterval is the cadence at which the stats observer samples
// the overflow counters.
const DefaultObserverInterval = time.Second

// StartObserver launches the low-frequency stats observer goroutine (P2-3).
//
// High-frequency conditions on the hot path — event drops, overflow waits,
// unrouted commands — are recorded as atomic counters where they occur; this
// goroutine is the only place they reach the text log. Every interval it
// samples the counters and emits one Warn line if anything changed since the
// previous sample (silent otherwise, so a healthy system logs nothing).
//
// The observer stops when ctx is cancelled. interval <= 0 selects
// DefaultObserverInterval.
func (m *MsgBus) StartObserver(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultObserverInterval
	}
	go m.observe(ctx, interval)
}

// observerSnapshot is one sample of every hot-path counter.
type observerSnapshot struct {
	drops    [event.TopicCount]uint64
	waits    [event.TopicCount]uint64
	unrouted uint64
}

func (m *MsgBus) sampleCounters(s *observerSnapshot) {
	for t := event.Topic(0); t < event.TopicCount; t++ {
		s.drops[t] = m.DropCount(t)
		s.waits[t] = m.WaitCount(t)
	}
	s.unrouted = m.UnroutedCommandCount()
}

func (m *MsgBus) observe(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prev, cur observerSnapshot
	m.sampleCounters(&prev)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sampleCounters(&cur)
			m.reportDelta(&prev, &cur, interval)
			if m.msgLogger != nil {
				_ = m.msgLogger.Sync()
			}
			prev = cur
		}
	}
}

// reportDelta logs one line per topic whose counters moved during the sample
// window, plus one line for unrouted commands. A healthy steady state emits
// nothing.
func (m *MsgBus) reportDelta(prev, cur *observerSnapshot, interval time.Duration) {
	for t := event.Topic(0); t < event.TopicCount; t++ {
		drops := cur.drops[t] - prev.drops[t]
		waits := cur.waits[t] - prev.waits[t]
		if drops == 0 && waits == 0 {
			continue
		}
		log().Warn().
			Str("topic", t.String()).
			Uint64("dropped", drops).
			Uint64("waited", waits).
			Uint64("dropped_total", cur.drops[t]).
			Uint64("waited_total", cur.waits[t]).
			Dur("window", interval).
			Msg("MsgBus: event overflow in sample window")
	}
	if unrouted := cur.unrouted - prev.unrouted; unrouted != 0 {
		log().Warn().
			Uint64("unrouted", unrouted).
			Uint64("unrouted_total", cur.unrouted).
			Dur("window", interval).
			Msg("MsgBus: commands with no registered processor in sample window")
	}
}
