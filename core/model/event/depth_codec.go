package event

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/common"
)

// Exported layout constants for direct buffer manipulation (used by HTTP adapters).
const (
	PriceLevelSize          = int(unsafe.Sizeof(common.PriceLevel{}))
	DepthSnapshotHeaderSize = 32 // SymbolID(8)+DepthID(8)+Timestamp(8)+AsksLen(4)+BidsLen(4)
	DepthUpdateHeaderSize   = 56 // SymbolID(8)+PreviousDepthID(8)+DepthID(8)+CurrentDepthID(8)+NextDepthID(8)+Timestamp(8)+AsksLen(4)+BidsLen(4)
)

// ============================================================================
// DepthSnapshot
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a DepthSnapshot.
func (d DepthSnapshot) GetBufferLength() int {
	return DepthSnapshotHeaderSize +
		len(d.Asks)*PriceLevelSize +
		len(d.Bids)*PriceLevelSize
}

// Encode writes the DepthSnapshot into buf.
// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
func (d DepthSnapshot) Encode(buf []byte) error {
	needed := d.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	asksLen := uint32(len(d.Asks))
	bidsLen := uint32(len(d.Bids))
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.SymbolID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.DepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], d.Timestamp)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	for i := range d.Asks {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&d.Asks[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	for i := range d.Bids {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&d.Bids[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	return nil
}

// NewDepthSnapshotFromBytes interprets buf as a DepthSnapshot. Zero-copy for slices.
func NewDepthSnapshotFromBytes(buf []byte) DepthSnapshot {
	pos := 0

	symbolID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	depthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	timestamp := binary.LittleEndian.Uint64(buf[pos:])
	pos += 8
	asksLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4
	bidsLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4

	var asks []common.PriceLevel
	var bids []common.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * PriceLevelSize
	}
	if bidsLen > 0 {
		bids = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
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

// Encode writes the DepthUpdate into buf.
// Layout: [SymbolID(8)][PreviousDepthID(8)][DepthID(8)][CurrentDepthID(8)][NextDepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
func (d DepthUpdate) Encode(buf []byte) error {
	needed := d.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	asksLen := uint32(len(d.Asks))
	bidsLen := uint32(len(d.Bids))
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.SymbolID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.PreviousDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.DepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.CurrentDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.NextDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], d.Timestamp)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	for i := range d.Asks {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&d.Asks[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	for i := range d.Bids {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&d.Bids[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	return nil
}

// NewDepthUpdateFromBytes interprets buf as a DepthUpdate. Zero-copy for slices.
func NewDepthUpdateFromBytes(buf []byte) DepthUpdate {
	pos := 0

	symbolID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	previousDepthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	depthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	currentDepthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	nextDepthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	timestamp := binary.LittleEndian.Uint64(buf[pos:])
	pos += 8
	asksLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4
	bidsLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4

	var asks []common.PriceLevel
	var bids []common.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * PriceLevelSize
	}
	if bidsLen > 0 {
		bids = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return DepthUpdate{
		SymbolID:        symbolID,
		PreviousDepthID: previousDepthID,
		DepthID:         depthID,
		CurrentDepthID:  currentDepthID,
		NextDepthID:     nextDepthID,
		Timestamp:       timestamp,
		Asks:            asks,
		Bids:            bids,
	}
}

// ============================================================================
// RespDepthSnapshot
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a RespDepthSnapshot.
func (r RespDepthSnapshot) GetBufferLength() int {
	return DepthSnapshotHeaderSize +
		len(r.Asks)*PriceLevelSize +
		len(r.Bids)*PriceLevelSize
}

// Encode writes the RespDepthSnapshot into buf. Uses the same layout as DepthSnapshot.
func (r RespDepthSnapshot) Encode(buf []byte) error {
	needed := r.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	asksLen := uint32(len(r.Asks))
	bidsLen := uint32(len(r.Bids))
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.SymbolID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.DepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], r.Timestamp)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	for i := range r.Asks {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&r.Asks[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	for i := range r.Bids {
		plBytes := (*[PriceLevelSize]byte)(unsafe.Pointer(&r.Bids[i]))[:]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	return nil
}

// NewRespDepthSnapshotFromBytes interprets buf as a RespDepthSnapshot. Zero-copy for slices.
func NewRespDepthSnapshotFromBytes(buf []byte) RespDepthSnapshot {
	pos := 0

	symbolID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	depthID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	timestamp := binary.LittleEndian.Uint64(buf[pos:])
	pos += 8
	asksLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4
	bidsLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4

	var asks []common.PriceLevel
	var bids []common.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * PriceLevelSize
	}
	if bidsLen > 0 {
		bids = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return RespDepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
}
