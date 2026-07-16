package event

import "github.com/BullionBear/seq/core/model/codec"

// Engine state event codecs, delegated to the generic bounds-checked memcpy
// pair in core/model/codec. Layout is guarded in codec/guard_test.go.

func (r ReadyEvent) GetBufferLength() int    { return codec.Size[ReadyEvent]() }
func (r ReadyEvent) Encode(buf []byte) error { return codec.Encode(buf, &r) }
func NewReadyEventFromBytes(buf []byte) (ReadyEvent, error) {
	return codec.Decode[ReadyEvent](buf)
}

func (s StopEvent) GetBufferLength() int    { return codec.Size[StopEvent]() }
func (s StopEvent) Encode(buf []byte) error { return codec.Encode(buf, &s) }
func NewStopEventFromBytes(buf []byte) (StopEvent, error) {
	return codec.Decode[StopEvent](buf)
}

func (f FinishedEvent) GetBufferLength() int    { return codec.Size[FinishedEvent]() }
func (f FinishedEvent) Encode(buf []byte) error { return codec.Encode(buf, &f) }
func NewFinishedEventFromBytes(buf []byte) (FinishedEvent, error) {
	return codec.Decode[FinishedEvent](buf)
}

func (a AbnormalEvent) GetBufferLength() int    { return codec.Size[AbnormalEvent]() }
func (a AbnormalEvent) Encode(buf []byte) error { return codec.Encode(buf, &a) }
func NewAbnormalEventFromBytes(buf []byte) (AbnormalEvent, error) {
	return codec.Decode[AbnormalEvent](buf)
}
