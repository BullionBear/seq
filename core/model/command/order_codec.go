package command

import "github.com/BullionBear/seq/core/model/codec"

// ErrBufferTooSmall is returned when the provided buffer is too small for
// encoding or decoding. It is the same error value as codec.ErrBufferTooSmall
// so identity comparisons work across packages.
var ErrBufferTooSmall = codec.ErrBufferTooSmall

// Order command codecs, delegated to the generic bounds-checked memcpy pair
// in core/model/codec. Layout is guarded in codec/guard_test.go.

func (r RiskCheck) GetBufferLength() int    { return codec.Size[RiskCheck]() }
func (r RiskCheck) Encode(buf []byte) error { return codec.Encode(buf, &r) }
func NewRiskCheckFromBytes(buf []byte) (RiskCheck, error) {
	return codec.Decode[RiskCheck](buf)
}

func (s SubmitOrder) GetBufferLength() int    { return codec.Size[SubmitOrder]() }
func (s SubmitOrder) Encode(buf []byte) error { return codec.Encode(buf, &s) }
func NewSubmitOrderFromBytes(buf []byte) (SubmitOrder, error) {
	return codec.Decode[SubmitOrder](buf)
}

func (c CancelOrder) GetBufferLength() int    { return codec.Size[CancelOrder]() }
func (c CancelOrder) Encode(buf []byte) error { return codec.Encode(buf, &c) }
func NewCancelOrderFromBytes(buf []byte) (CancelOrder, error) {
	return codec.Decode[CancelOrder](buf)
}

func (c CancelAll) GetBufferLength() int    { return codec.Size[CancelAll]() }
func (c CancelAll) Encode(buf []byte) error { return codec.Encode(buf, &c) }
func NewCancelAllFromBytes(buf []byte) (CancelAll, error) {
	return codec.Decode[CancelAll](buf)
}
