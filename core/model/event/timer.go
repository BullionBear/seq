package event

import "github.com/BullionBear/seq/core/model/codec"

// TimeEvent is published by the Clock on each timer tick.
// TimerID identifies which registered timer fired.
// ScheduledNs is the scheduled fire time (not wall-clock).
type TimeEvent struct {
	TimerID     uint64
	ScheduledNs uint64
}

func (t TimeEvent) Topic() Topic { return TopicEventTimer }

func (t TimeEvent) GetBufferLength() int    { return codec.Size[TimeEvent]() }
func (t TimeEvent) Encode(buf []byte) error { return codec.Encode(buf, &t) }
func NewTimeEventFromBytes(buf []byte) (TimeEvent, error) {
	return codec.Decode[TimeEvent](buf)
}
