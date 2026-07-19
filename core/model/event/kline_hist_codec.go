package event

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/codec"
	"github.com/BullionBear/seq/core/model/common"
)

// Wire header for RespHistoricalKline:
// [SymbolID(8)][Interval(8)][BarsLen(4)][pad(4)][KlineBar...]
type respHistoricalKlineHeader struct {
	SymbolID uint64
	Interval uint64
	BarsLen  uint32
	_        [4]byte
}

const (
	KlineBarSize                   = int(unsafe.Sizeof(KlineBar{}))
	RespHistoricalKlineHeaderSize  = int(unsafe.Sizeof(respHistoricalKlineHeader{}))
)

func validKlineBarCount(bufLen, headerSize int, barsLen uint32) bool {
	need := uint64(headerSize) + uint64(barsLen)*uint64(KlineBarSize)
	return uint64(bufLen) >= need
}

func klineBarSlice(buf []byte, base, n int) []KlineBar {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*KlineBar)(unsafe.Pointer(&buf[base])), n)
}

// GetBufferLength returns the number of bytes needed to encode a RespHistoricalKline.
func (r RespHistoricalKline) GetBufferLength() int {
	return RespHistoricalKlineHeaderSize + len(r.Bars)*KlineBarSize
}

// Encode writes the RespHistoricalKline into buf.
func (r RespHistoricalKline) Encode(buf []byte) error {
	if len(buf) < r.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(r.SymbolID))
	c.PutUint64(uint64(r.Interval))
	c.PutUint32(uint32(len(r.Bars)))
	c.PutUint32(0) // padding
	for i := range r.Bars {
		codec.Put(&c, &r.Bars[i])
	}
	return c.Err()
}

// NewRespHistoricalKlineFromBytes interprets buf as a RespHistoricalKline.
// Bars is a zero-copy view into buf and is only valid while buf is.
func NewRespHistoricalKlineFromBytes(buf []byte) (RespHistoricalKline, error) {
	if len(buf) < RespHistoricalKlineHeaderSize {
		return RespHistoricalKline{}, ErrBufferTooSmall
	}
	symbolID := int(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(respHistoricalKlineHeader{}.SymbolID):]))
	interval := common.Interval(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(respHistoricalKlineHeader{}.Interval):]))
	barsLen := binary.LittleEndian.Uint32(buf[unsafe.Offsetof(respHistoricalKlineHeader{}.BarsLen):])

	if !validKlineBarCount(len(buf), RespHistoricalKlineHeaderSize, barsLen) {
		return RespHistoricalKline{}, ErrInvalidBuffer
	}

	return RespHistoricalKline{
		SymbolID: symbolID,
		Interval: interval,
		Bars:     klineBarSlice(buf, RespHistoricalKlineHeaderSize, int(barsLen)),
	}, nil
}
