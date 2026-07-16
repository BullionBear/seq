package event

import "unsafe"

// TimeEvent is published by the Clock on each timer tick.
// TimerID identifies which registered timer fired.
// ScheduledNs is the scheduled fire time (not wall-clock).
type TimeEvent struct {
	TimerID     uint64
	ScheduledNs uint64
}

func (t TimeEvent) Topic() Topic { return TopicEventTimer }

const sizeOfTimeEvent = int(unsafe.Sizeof(TimeEvent{}))

func (t TimeEvent) GetBufferLength() int { return sizeOfTimeEvent }

func (t TimeEvent) Encode(buf []byte) error {
	if len(buf) < sizeOfTimeEvent {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfTimeEvent]byte)(unsafe.Pointer(&t))[:]
	copy(buf, data)
	return nil
}

func NewTimeEventFromBytes(buf []byte) (TimeEvent, error) {
	var v TimeEvent
	if len(buf) < sizeOfTimeEvent {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfTimeEvent]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
}
