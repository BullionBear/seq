package event

import (
	"unsafe"

	"github.com/BullionBear/seq/core/model/codec"
	"github.com/BullionBear/seq/core/model/common"
)

// Wire headers for the variable-size depth events. These structs declare the
// layout: header sizes and field offsets below are derived from them with
// unsafe.Sizeof/Offsetof — no hand-computed byte counts. All integer fields
// are written little-endian; the level arrays follow the header as raw
// common.PriceLevel images (asks first, then bids).
type depthSnapshotHeader struct {
	SymbolID  uint64
	DepthID   uint64
	Timestamp uint64
	AsksLen   uint32
	BidsLen   uint32
}

type depthUpdateHeader struct {
	SymbolID        uint64
	PreviousDepthID uint64
	DepthID         uint64
	CurrentDepthID  uint64
	NextDepthID     uint64
	Timestamp       uint64
	AsksLen         uint32
	BidsLen         uint32
}

// Exported layout constants for direct buffer manipulation (used by HTTP adapters).
const (
	PriceLevelSize          = int(unsafe.Sizeof(common.PriceLevel{}))
	DepthSnapshotHeaderSize = int(unsafe.Sizeof(depthSnapshotHeader{}))
	DepthUpdateHeaderSize   = int(unsafe.Sizeof(depthUpdateHeader{}))
)

// Derived field offsets used by the flyweight views.
const (
	snapSymbolIDOff  = int(unsafe.Offsetof(depthSnapshotHeader{}.SymbolID))
	snapDepthIDOff   = int(unsafe.Offsetof(depthSnapshotHeader{}.DepthID))
	snapTimestampOff = int(unsafe.Offsetof(depthSnapshotHeader{}.Timestamp))
	snapAsksLenOff   = int(unsafe.Offsetof(depthSnapshotHeader{}.AsksLen))
	snapBidsLenOff   = int(unsafe.Offsetof(depthSnapshotHeader{}.BidsLen))

	updSymbolIDOff        = int(unsafe.Offsetof(depthUpdateHeader{}.SymbolID))
	updPreviousDepthIDOff = int(unsafe.Offsetof(depthUpdateHeader{}.PreviousDepthID))
	updDepthIDOff         = int(unsafe.Offsetof(depthUpdateHeader{}.DepthID))
	updCurrentDepthIDOff  = int(unsafe.Offsetof(depthUpdateHeader{}.CurrentDepthID))
	updNextDepthIDOff     = int(unsafe.Offsetof(depthUpdateHeader{}.NextDepthID))
	updTimestampOff       = int(unsafe.Offsetof(depthUpdateHeader{}.Timestamp))
	updAsksLenOff         = int(unsafe.Offsetof(depthUpdateHeader{}.AsksLen))
	updBidsLenOff         = int(unsafe.Offsetof(depthUpdateHeader{}.BidsLen))
)

// validLevelCounts checks the header + n*PriceLevelSize length invariant
// without integer overflow: counts are widened to uint64 before multiplying.
func validLevelCounts(bufLen, headerSize int, asksLen, bidsLen uint32) bool {
	need := uint64(headerSize) + (uint64(asksLen)+uint64(bidsLen))*uint64(PriceLevelSize)
	return uint64(bufLen) >= need
}

// encodeSnapshotLayout writes the shared DepthSnapshot/RespDepthSnapshot
// layout: [header][Asks...][Bids...].
func encodeSnapshotLayout(buf []byte, symbolID, depthID int, timestamp uint64, asks, bids []common.PriceLevel) error {
	needed := DepthSnapshotHeaderSize + (len(asks)+len(bids))*PriceLevelSize
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(symbolID))
	c.PutUint64(uint64(depthID))
	c.PutUint64(timestamp)
	c.PutUint32(uint32(len(asks)))
	c.PutUint32(uint32(len(bids)))
	for i := range asks {
		codec.Put(&c, &asks[i])
	}
	for i := range bids {
		codec.Put(&c, &bids[i])
	}
	return c.Err()
}

// ============================================================================
// DepthSnapshot
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a DepthSnapshot.
func (d DepthSnapshot) GetBufferLength() int {
	return DepthSnapshotHeaderSize +
		len(d.Asks)*PriceLevelSize +
		len(d.Bids)*PriceLevelSize
}

// Encode writes the DepthSnapshot into buf (snapshot layout, see depthSnapshotHeader).
func (d DepthSnapshot) Encode(buf []byte) error {
	return encodeSnapshotLayout(buf, d.SymbolID, d.DepthID, d.Timestamp, d.Asks, d.Bids)
}

// NewDepthSnapshotFromBytes interprets buf as a DepthSnapshot.
// The buffer length is validated against the header-declared level counts
// before any read. The Asks/Bids slices are zero-copy views into buf and are
// only valid while buf is (i.e. within the dispatch handler, before the
// event's arena reservation is released). Prefer NewDepthSnapshotView, which
// avoids materializing the slices at all.
func NewDepthSnapshotFromBytes(buf []byte) (DepthSnapshot, error) {
	v, err := NewDepthSnapshotView(buf)
	if err != nil {
		return DepthSnapshot{}, err
	}
	return DepthSnapshot{
		SymbolID:  v.SymbolID(),
		DepthID:   v.DepthID(),
		Timestamp: v.Timestamp(),
		Asks:      v.Asks(),
		Bids:      v.Bids(),
	}, nil
}

// ============================================================================
// DepthUpdate
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a DepthUpdate.
func (d DepthUpdate) GetBufferLength() int {
	return DepthUpdateHeaderSize +
		len(d.Asks)*PriceLevelSize +
		len(d.Bids)*PriceLevelSize
}

// Encode writes the DepthUpdate into buf (update layout, see depthUpdateHeader).
func (d DepthUpdate) Encode(buf []byte) error {
	if len(buf) < d.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(d.SymbolID))
	c.PutUint64(uint64(d.PreviousDepthID))
	c.PutUint64(uint64(d.DepthID))
	c.PutUint64(uint64(d.CurrentDepthID))
	c.PutUint64(uint64(d.NextDepthID))
	c.PutUint64(d.Timestamp)
	c.PutUint32(uint32(len(d.Asks)))
	c.PutUint32(uint32(len(d.Bids)))
	for i := range d.Asks {
		codec.Put(&c, &d.Asks[i])
	}
	for i := range d.Bids {
		codec.Put(&c, &d.Bids[i])
	}
	return c.Err()
}

// NewDepthUpdateFromBytes interprets buf as a DepthUpdate.
// The buffer length is validated against the header-declared level counts
// before any read. The Asks/Bids slices are zero-copy views into buf and are
// only valid while buf is (i.e. within the dispatch handler, before the
// event's arena reservation is released). Prefer NewDepthUpdateView, which
// avoids materializing the slices at all.
func NewDepthUpdateFromBytes(buf []byte) (DepthUpdate, error) {
	v, err := NewDepthUpdateView(buf)
	if err != nil {
		return DepthUpdate{}, err
	}
	return DepthUpdate{
		SymbolID:        v.SymbolID(),
		PreviousDepthID: v.PreviousDepthID(),
		DepthID:         v.DepthID(),
		CurrentDepthID:  v.CurrentDepthID(),
		NextDepthID:     v.NextDepthID(),
		Timestamp:       v.Timestamp(),
		Asks:            v.Asks(),
		Bids:            v.Bids(),
	}, nil
}

// ============================================================================
// RespDepthSnapshot (same wire layout as DepthSnapshot)
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a RespDepthSnapshot.
func (r RespDepthSnapshot) GetBufferLength() int {
	return DepthSnapshotHeaderSize +
		len(r.Asks)*PriceLevelSize +
		len(r.Bids)*PriceLevelSize
}

// Encode writes the RespDepthSnapshot into buf. Uses the same layout as DepthSnapshot.
func (r RespDepthSnapshot) Encode(buf []byte) error {
	return encodeSnapshotLayout(buf, r.SymbolID, r.DepthID, r.Timestamp, r.Asks, r.Bids)
}

// NewRespDepthSnapshotFromBytes interprets buf as a RespDepthSnapshot.
// Same validation and slice-lifetime contract as NewDepthSnapshotFromBytes.
func NewRespDepthSnapshotFromBytes(buf []byte) (RespDepthSnapshot, error) {
	v, err := NewDepthSnapshotView(buf)
	if err != nil {
		return RespDepthSnapshot{}, err
	}
	return RespDepthSnapshot{
		SymbolID:  v.SymbolID(),
		DepthID:   v.DepthID(),
		Timestamp: v.Timestamp(),
		AskLength: v.NumAsks(),
		BidLength: v.NumBids(),
		Asks:      v.Asks(),
		Bids:      v.Bids(),
	}, nil
}
