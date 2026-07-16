package event

import "github.com/BullionBear/seq/core/model/codec"

// Fixed-size order event codecs, delegated to the generic bounds-checked
// memcpy pair in core/model/codec. Layout is guarded in codec/guard_test.go.

func (o OrderNew) GetBufferLength() int    { return codec.Size[OrderNew]() }
func (o OrderNew) Encode(buf []byte) error { return codec.Encode(buf, &o) }
func NewOrderNewFromBytes(buf []byte) (OrderNew, error) {
	return codec.Decode[OrderNew](buf)
}

func (o OrderAccepted) GetBufferLength() int    { return codec.Size[OrderAccepted]() }
func (o OrderAccepted) Encode(buf []byte) error { return codec.Encode(buf, &o) }
func NewOrderAcceptedFromBytes(buf []byte) (OrderAccepted, error) {
	return codec.Decode[OrderAccepted](buf)
}

func (o OrderPartiallyFilled) GetBufferLength() int    { return codec.Size[OrderPartiallyFilled]() }
func (o OrderPartiallyFilled) Encode(buf []byte) error { return codec.Encode(buf, &o) }
func NewOrderPartiallyFilledFromBytes(buf []byte) (OrderPartiallyFilled, error) {
	return codec.Decode[OrderPartiallyFilled](buf)
}

func (o OrderFilled) GetBufferLength() int    { return codec.Size[OrderFilled]() }
func (o OrderFilled) Encode(buf []byte) error { return codec.Encode(buf, &o) }
func NewOrderFilledFromBytes(buf []byte) (OrderFilled, error) {
	return codec.Decode[OrderFilled](buf)
}

func (o OrderCanceled) GetBufferLength() int    { return codec.Size[OrderCanceled]() }
func (o OrderCanceled) Encode(buf []byte) error { return codec.Encode(buf, &o) }
func NewOrderCanceledFromBytes(buf []byte) (OrderCanceled, error) {
	return codec.Decode[OrderCanceled](buf)
}

func (e Execution) GetBufferLength() int    { return codec.Size[Execution]() }
func (e Execution) Encode(buf []byte) error { return codec.Encode(buf, &e) }
func NewExecutionFromBytes(buf []byte) (Execution, error) {
	return codec.Decode[Execution](buf)
}
