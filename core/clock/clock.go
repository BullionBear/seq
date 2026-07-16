package clock

import (
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Clock manages periodic timers that fire callbacks and publish TimeEvents
// to the msgbus. All methods are called from the single dispatch thread.
type Clock struct {
	msgBus *msgbus.MsgBus
	timers map[uint64]*timerEntry
	nextID uint64
}

type timerEntry struct {
	id         uint64
	intervalNs uint64
	nextFireNs uint64
	callback   func(event.TimeEvent)
}

// CancelToken is returned by Register and allows the caller to cancel the timer.
type CancelToken struct {
	id    uint64
	clock *Clock
}

// Cancel removes the timer from the clock. Safe to call multiple times.
func (t *CancelToken) Cancel() {
	delete(t.clock.timers, t.id)
}

// NewClock creates a new Clock attached to the given MsgBus.
func NewClock(bus *msgbus.MsgBus) *Clock {
	return &Clock{
		msgBus: bus,
		timers: make(map[uint64]*timerEntry),
		nextID: 0,
	}
}

// Register creates a periodic timer that first fires at startNs, then every
// intervalNs nanoseconds. The callback is invoked directly from the dispatch
// thread during Tick(). A TimeEvent is also published to the msgbus.
// Returns a CancelToken to stop the timer.
func (c *Clock) Register(startNs, intervalNs uint64, callback func(event.TimeEvent)) *CancelToken {
	id := c.nextID
	c.nextID++
	c.timers[id] = &timerEntry{
		id:         id,
		intervalNs: intervalNs,
		nextFireNs: startNs,
		callback:   callback,
	}
	log().Debug().
		Uint64("timer_id", id).
		Uint64("start_ns", startNs).
		Uint64("interval_ns", intervalNs).
		Msg("Clock: timer registered")
	return &CancelToken{id: id, clock: c}
}

// Tick checks all registered timers against nowNs and fires any that are due.
// For each due timer it calls the callback and publishes a TimeEvent.
// If multiple intervals have elapsed, each tick is fired individually.
func (c *Clock) Tick(nowNs uint64) {
	for _, t := range c.timers {
		for nowNs >= t.nextFireNs {
			ev := event.TimeEvent{
				TimerID:     t.id,
				ScheduledNs: t.nextFireNs,
			}

			t.callback(ev)

			if ref, buf, ok := c.msgBus.Allocate(event.TopicEventTimer, uint64(ev.GetBufferLength())); ok {
				if err := ev.Encode(buf); err != nil {
					c.msgBus.Cancel(ref)
				} else {
					c.msgBus.Publish(ref)
				}
			}

			t.nextFireNs += t.intervalNs
		}
	}
}

// TimerCount returns the number of active timers.
func (c *Clock) TimerCount() int {
	return len(c.timers)
}
