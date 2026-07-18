package codec

import (
	"errors"
	"unsafe"
)

// ErrBufferTooSmall is returned when the provided buffer cannot hold (Encode)
// or provide (Decode) the full wire representation of the value.
// The message deliberately carries no package prefix: it predates this
// package (event/command re-export the same value) and is pinned by the
// msglog JSONL golden in core/msgbus.
var ErrBufferTooSmall = errors.New("buffer too small")

// Size returns the wire size of T: its in-memory size including padding.
// The layout is a frozen contract enforced by the guard tests in this package.
func Size[T any]() int {
	var v T
	return int(unsafe.Sizeof(v))
}

// Encode writes the raw memory of *v into buf (bounds-checked memcpy).
// T must be a registered POD wire type (see guard_test.go): no pointers,
// slices, maps, strings, chans, funcs, or interfaces anywhere in the struct.
func Encode[T any](buf []byte, v *T) error {
	size := int(unsafe.Sizeof(*v))
	if len(buf) < size {
		return ErrBufferTooSmall
	}
	copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(v)), size))
	return nil
}

// Decode copies buf into a new local value of T (bounds-checked memcpy).
// The returned value does not alias buf, so it stays valid after the buffer
// is released, and the copy works from unaligned source offsets.
func Decode[T any](buf []byte) (T, error) {
	var v T
	size := int(unsafe.Sizeof(v))
	if len(buf) < size {
		return v, ErrBufferTooSmall
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&v)), size), buf)
	return v, nil
}
