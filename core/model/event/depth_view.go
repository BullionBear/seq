package event

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/common"
)

// Flyweight read views over encoded depth events. A view holds only the
// encoded buffer and decodes fields on access — no materialization, no
// allocation. Views are valid exactly as long as the underlying buffer is
// (i.e. within the dispatch handler, before the event's arena reservation
// is released); copy any level that must outlive the handler.

// DepthSnapshotView is a zero-copy view over an encoded DepthSnapshot or
// RespDepthSnapshot (they share the same wire layout, depthSnapshotHeader).
type DepthSnapshotView struct {
	buf []byte
}

// NewDepthSnapshotView validates the minimum length and the
// header + n*PriceLevelSize invariant, then wraps buf without copying.
func NewDepthSnapshotView(buf []byte) (DepthSnapshotView, error) {
	if len(buf) < DepthSnapshotHeaderSize {
		return DepthSnapshotView{}, ErrBufferTooSmall
	}
	asksLen := binary.LittleEndian.Uint32(buf[snapAsksLenOff:])
	bidsLen := binary.LittleEndian.Uint32(buf[snapBidsLenOff:])
	if !validLevelCounts(len(buf), DepthSnapshotHeaderSize, asksLen, bidsLen) {
		return DepthSnapshotView{}, ErrInvalidBuffer
	}
	return DepthSnapshotView{buf: buf}, nil
}

func (v DepthSnapshotView) SymbolID() int {
	return int(binary.LittleEndian.Uint64(v.buf[snapSymbolIDOff:]))
}

func (v DepthSnapshotView) DepthID() int {
	return int(binary.LittleEndian.Uint64(v.buf[snapDepthIDOff:]))
}

func (v DepthSnapshotView) Timestamp() uint64 {
	return binary.LittleEndian.Uint64(v.buf[snapTimestampOff:])
}

func (v DepthSnapshotView) NumAsks() int {
	return int(binary.LittleEndian.Uint32(v.buf[snapAsksLenOff:]))
}

func (v DepthSnapshotView) NumBids() int {
	return int(binary.LittleEndian.Uint32(v.buf[snapBidsLenOff:]))
}

// Ask returns the i-th ask level by value (copied out of the buffer).
// Panics if i is out of [0, NumAsks()).
func (v DepthSnapshotView) Ask(i int) common.PriceLevel {
	return levelAt(v.buf, DepthSnapshotHeaderSize, i, v.NumAsks())
}

// Bid returns the i-th bid level by value (copied out of the buffer).
// Panics if i is out of [0, NumBids()).
func (v DepthSnapshotView) Bid(i int) common.PriceLevel {
	return levelAt(v.buf, DepthSnapshotHeaderSize+v.NumAsks()*PriceLevelSize, i, v.NumBids())
}

// Asks returns a zero-copy slice aliasing the buffer's ask levels.
// Same lifetime contract as the view itself.
func (v DepthSnapshotView) Asks() []common.PriceLevel {
	return levelSlice(v.buf, DepthSnapshotHeaderSize, v.NumAsks())
}

// Bids returns a zero-copy slice aliasing the buffer's bid levels.
// Same lifetime contract as the view itself.
func (v DepthSnapshotView) Bids() []common.PriceLevel {
	return levelSlice(v.buf, DepthSnapshotHeaderSize+v.NumAsks()*PriceLevelSize, v.NumBids())
}

// DepthUpdateView is a zero-copy view over an encoded DepthUpdate
// (wire layout: depthUpdateHeader).
type DepthUpdateView struct {
	buf []byte
}

// NewDepthUpdateView validates the minimum length and the
// header + n*PriceLevelSize invariant, then wraps buf without copying.
func NewDepthUpdateView(buf []byte) (DepthUpdateView, error) {
	if len(buf) < DepthUpdateHeaderSize {
		return DepthUpdateView{}, ErrBufferTooSmall
	}
	asksLen := binary.LittleEndian.Uint32(buf[updAsksLenOff:])
	bidsLen := binary.LittleEndian.Uint32(buf[updBidsLenOff:])
	if !validLevelCounts(len(buf), DepthUpdateHeaderSize, asksLen, bidsLen) {
		return DepthUpdateView{}, ErrInvalidBuffer
	}
	return DepthUpdateView{buf: buf}, nil
}

func (v DepthUpdateView) SymbolID() int {
	return int(binary.LittleEndian.Uint64(v.buf[updSymbolIDOff:]))
}

func (v DepthUpdateView) PreviousDepthID() int {
	return int(binary.LittleEndian.Uint64(v.buf[updPreviousDepthIDOff:]))
}

func (v DepthUpdateView) DepthID() int {
	return int(binary.LittleEndian.Uint64(v.buf[updDepthIDOff:]))
}

func (v DepthUpdateView) CurrentDepthID() int {
	return int(binary.LittleEndian.Uint64(v.buf[updCurrentDepthIDOff:]))
}

func (v DepthUpdateView) NextDepthID() int {
	return int(binary.LittleEndian.Uint64(v.buf[updNextDepthIDOff:]))
}

func (v DepthUpdateView) Timestamp() uint64 {
	return binary.LittleEndian.Uint64(v.buf[updTimestampOff:])
}

func (v DepthUpdateView) NumAsks() int {
	return int(binary.LittleEndian.Uint32(v.buf[updAsksLenOff:]))
}

func (v DepthUpdateView) NumBids() int {
	return int(binary.LittleEndian.Uint32(v.buf[updBidsLenOff:]))
}

// Ask returns the i-th ask level by value (copied out of the buffer).
// Panics if i is out of [0, NumAsks()).
func (v DepthUpdateView) Ask(i int) common.PriceLevel {
	return levelAt(v.buf, DepthUpdateHeaderSize, i, v.NumAsks())
}

// Bid returns the i-th bid level by value (copied out of the buffer).
// Panics if i is out of [0, NumBids()).
func (v DepthUpdateView) Bid(i int) common.PriceLevel {
	return levelAt(v.buf, DepthUpdateHeaderSize+v.NumAsks()*PriceLevelSize, i, v.NumBids())
}

// Asks returns a zero-copy slice aliasing the buffer's ask levels.
// Same lifetime contract as the view itself.
func (v DepthUpdateView) Asks() []common.PriceLevel {
	return levelSlice(v.buf, DepthUpdateHeaderSize, v.NumAsks())
}

// Bids returns a zero-copy slice aliasing the buffer's bid levels.
// Same lifetime contract as the view itself.
func (v DepthUpdateView) Bids() []common.PriceLevel {
	return levelSlice(v.buf, DepthUpdateHeaderSize+v.NumAsks()*PriceLevelSize, v.NumBids())
}

// levelAt copies out the i-th PriceLevel of the array starting at base.
// The constructor already validated the total length; the explicit bound
// check here turns an out-of-range index into a clear panic instead of a
// read from the adjacent array.
func levelAt(buf []byte, base, i, n int) common.PriceLevel {
	if i < 0 || i >= n {
		panic("event: price level index out of range")
	}
	var pl common.PriceLevel
	off := base + i*PriceLevelSize
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&pl)), PriceLevelSize), buf[off:off+PriceLevelSize])
	return pl
}

// levelSlice aliases n PriceLevels starting at base. Arena reservations are
// 8-byte aligned and both header sizes are multiples of 8, so the cast is
// aligned.
func levelSlice(buf []byte, base, n int) []common.PriceLevel {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[base])), n)
}
