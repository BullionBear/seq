package event

import (
	"errors"

	"github.com/BullionBear/seq/core/model/codec"
)

// ErrBufferTooSmall is returned when the provided buffer is too small for
// encoding or decoding. It is the same error value as codec.ErrBufferTooSmall
// so identity comparisons work across packages.
var ErrBufferTooSmall = codec.ErrBufferTooSmall

// ErrInvalidBuffer is returned when a decoded buffer is structurally invalid
// (e.g. its length does not match the header-declared element counts).
var ErrInvalidBuffer = errors.New("invalid buffer layout")

// GetBufferLength returns the number of bytes needed to encode a Tick.
func (t Tick) GetBufferLength() int { return codec.Size[Tick]() }

// Encode writes the Tick into buf. Returns an error if buf is too small.
func (t Tick) Encode(buf []byte) error { return codec.Encode(buf, &t) }

// NewTickFromBytes decodes a Tick by copying out of buf (bounds-checked).
func NewTickFromBytes(buf []byte) (Tick, error) { return codec.Decode[Tick](buf) }
