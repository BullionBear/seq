package command

import (
	"errors"
	"unsafe"
)

// ErrBufferTooSmall is returned when the provided buffer is too small for encoding.
var ErrBufferTooSmall = errors.New("buffer too small")

const (
	sizeOfRiskCheck   = int(unsafe.Sizeof(RiskCheck{}))
	sizeOfSubmitOrder = int(unsafe.Sizeof(SubmitOrder{}))
	sizeOfCancelOrder = int(unsafe.Sizeof(CancelOrder{}))
	sizeOfCancelAll   = int(unsafe.Sizeof(CancelAll{}))
)

// ============================================================================
// RiskCheck
// ============================================================================

func (r RiskCheck) GetBufferLength() int { return sizeOfRiskCheck }

func (r RiskCheck) Encode(buf []byte) error {
	if len(buf) < sizeOfRiskCheck {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfRiskCheck]byte)(unsafe.Pointer(&r))[:]
	copy(buf, data)
	return nil
}

func NewRiskCheckFromBytes(buf []byte) RiskCheck {
	return *(*RiskCheck)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// SubmitOrder
// ============================================================================

func (s SubmitOrder) GetBufferLength() int { return sizeOfSubmitOrder }

func (s SubmitOrder) Encode(buf []byte) error {
	if len(buf) < sizeOfSubmitOrder {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfSubmitOrder]byte)(unsafe.Pointer(&s))[:]
	copy(buf, data)
	return nil
}

func NewSubmitOrderFromBytes(buf []byte) SubmitOrder {
	return *(*SubmitOrder)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// CancelOrder
// ============================================================================

func (c CancelOrder) GetBufferLength() int { return sizeOfCancelOrder }

func (c CancelOrder) Encode(buf []byte) error {
	if len(buf) < sizeOfCancelOrder {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfCancelOrder]byte)(unsafe.Pointer(&c))[:]
	copy(buf, data)
	return nil
}

func NewCancelOrderFromBytes(buf []byte) CancelOrder {
	return *(*CancelOrder)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// CancelAll
// ============================================================================

func (c CancelAll) GetBufferLength() int { return sizeOfCancelAll }

func (c CancelAll) Encode(buf []byte) error {
	if len(buf) < sizeOfCancelAll {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfCancelAll]byte)(unsafe.Pointer(&c))[:]
	copy(buf, data)
	return nil
}

func NewCancelAllFromBytes(buf []byte) CancelAll {
	return *(*CancelAll)(unsafe.Pointer(&buf[0]))
}
