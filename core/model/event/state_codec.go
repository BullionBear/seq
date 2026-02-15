package event

import "unsafe"

const (
	sizeOfReadyEvent    = int(unsafe.Sizeof(ReadyEvent{}))
	sizeOfStopEvent     = int(unsafe.Sizeof(StopEvent{}))
	sizeOfFinishedEvent = int(unsafe.Sizeof(FinishedEvent{}))
	sizeOfAbnormalEvent = int(unsafe.Sizeof(AbnormalEvent{}))
)

// ============================================================================
// ReadyEvent
// ============================================================================

func (r ReadyEvent) GetBufferLength() int { return sizeOfReadyEvent }

func (r ReadyEvent) Encode(buf []byte) error {
	if len(buf) < sizeOfReadyEvent {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfReadyEvent]byte)(unsafe.Pointer(&r))[:]
	copy(buf, data)
	return nil
}

func NewReadyEventFromBytes(buf []byte) ReadyEvent {
	return *(*ReadyEvent)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// StopEvent
// ============================================================================

func (s StopEvent) GetBufferLength() int { return sizeOfStopEvent }

func (s StopEvent) Encode(buf []byte) error {
	if len(buf) < sizeOfStopEvent {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfStopEvent]byte)(unsafe.Pointer(&s))[:]
	copy(buf, data)
	return nil
}

func NewStopEventFromBytes(buf []byte) StopEvent {
	return *(*StopEvent)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// FinishedEvent
// ============================================================================

func (f FinishedEvent) GetBufferLength() int { return sizeOfFinishedEvent }

func (f FinishedEvent) Encode(buf []byte) error {
	if len(buf) < sizeOfFinishedEvent {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfFinishedEvent]byte)(unsafe.Pointer(&f))[:]
	copy(buf, data)
	return nil
}

func NewFinishedEventFromBytes(buf []byte) FinishedEvent {
	return *(*FinishedEvent)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// AbnormalEvent
// ============================================================================

func (a AbnormalEvent) GetBufferLength() int { return sizeOfAbnormalEvent }

func (a AbnormalEvent) Encode(buf []byte) error {
	if len(buf) < sizeOfAbnormalEvent {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfAbnormalEvent]byte)(unsafe.Pointer(&a))[:]
	copy(buf, data)
	return nil
}

func NewAbnormalEventFromBytes(buf []byte) AbnormalEvent {
	return *(*AbnormalEvent)(unsafe.Pointer(&buf[0]))
}
