package event

import "github.com/BullionBear/seq/core/model/codec"

// GetBufferLength returns the number of bytes needed to encode a Kline.
func (k Kline) GetBufferLength() int { return codec.Size[Kline]() }

// Encode writes the Kline into buf. Returns an error if buf is too small.
func (k Kline) Encode(buf []byte) error { return codec.Encode(buf, &k) }

// NewKlineFromBytes decodes a Kline by copying out of buf (bounds-checked).
func NewKlineFromBytes(buf []byte) (Kline, error) { return codec.Decode[Kline](buf) }
