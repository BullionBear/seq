package event

import (
	"errors"
	"unsafe"
)

// ErrBufferTooSmall is returned when the provided buffer is too small for
// encoding or decoding.
var ErrBufferTooSmall = errors.New("buffer too small")

// ErrInvalidBuffer is returned when a decoded buffer is structurally invalid
// (e.g. its length does not match the header-declared element counts).
var ErrInvalidBuffer = errors.New("invalid buffer layout")

const sizeOfTick = int(unsafe.Sizeof(Tick{}))

// GetBufferLength returns the number of bytes needed to encode a Tick.
func (t Tick) GetBufferLength() int {
	return sizeOfTick
}

// Encode writes the Tick into buf. Returns an error if buf is too small.
func (t Tick) Encode(buf []byte) error {
	if len(buf) < sizeOfTick {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfTick]byte)(unsafe.Pointer(&t))[:]
	copy(buf, data)
	return nil
}

// NewTickFromBytes decodes a Tick by copying out of buf (bounds-checked).
func NewTickFromBytes(buf []byte) (Tick, error) {
	var v Tick
	if len(buf) < sizeOfTick {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfTick]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
}
