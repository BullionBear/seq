package event

import (
	"errors"
	"unsafe"
)

// ErrBufferTooSmall is returned when the provided buffer is too small for encoding.
var ErrBufferTooSmall = errors.New("buffer too small")

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

// NewTickFromBytes interprets buf as a Tick. Zero-copy, no allocation.
func NewTickFromBytes(buf []byte) Tick {
	return *(*Tick)(unsafe.Pointer(&buf[0]))
}
