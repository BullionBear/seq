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

func NewReadyEventFromBytes(buf []byte) (ReadyEvent, error) {
	var v ReadyEvent
	if len(buf) < sizeOfReadyEvent {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfReadyEvent]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewStopEventFromBytes(buf []byte) (StopEvent, error) {
	var v StopEvent
	if len(buf) < sizeOfStopEvent {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfStopEvent]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewFinishedEventFromBytes(buf []byte) (FinishedEvent, error) {
	var v FinishedEvent
	if len(buf) < sizeOfFinishedEvent {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfFinishedEvent]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewAbnormalEventFromBytes(buf []byte) (AbnormalEvent, error) {
	var v AbnormalEvent
	if len(buf) < sizeOfAbnormalEvent {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfAbnormalEvent]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
}
