package evbus

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/event"
)

// Size constants for fixed-size event types
const (
	SizeOfTick        = int(unsafe.Sizeof(event.Tick{}))
	SizeOfOrderUpdate = int(unsafe.Sizeof(event.OrderUpdate{}))
	SizeOfFill        = int(unsafe.Sizeof(event.Fill{}))
	SizeOfPriceLevel  = int(unsafe.Sizeof(event.PriceLevel{}))

	// Header sizes for variable-size events (without slice data)
	// DepthUpdate: SymbolID(8) + PreviousDepthID(8) + DepthID(8) + CurrentDepthID(8) + NextDepthID(8) + Timestamp(8) = 48 bytes
	// Plus AsksLen(4) + BidsLen(4) = 8 bytes for length prefix
	SizeOfDepthUpdateHeader = 48 + 8

	// DepthSnapshot: SymbolID(8) + DepthID(8) + Timestamp(8) = 24 bytes
	// Plus AsksLen(4) + BidsLen(4) = 8 bytes for length prefix
	SizeOfDepthSnapshotHeader = 24 + 8
)

// TickSize returns the size needed to serialize a Tick event.
func TickSize() uint64 {
	return uint64(SizeOfTick)
}

// SerializeTick writes a Tick to the buffer using unsafe pointer casting.
// Returns the number of bytes written.
func SerializeTick(buf []byte, tick *event.Tick) int {
	data := (*[SizeOfTick]byte)(unsafe.Pointer(tick))[:]
	copy(buf, data)
	return SizeOfTick
}

// DeserializeTick reads a Tick from buffer using unsafe pointer casting.
func DeserializeTick(buf []byte) event.Tick {
	return *(*event.Tick)(unsafe.Pointer(&buf[0]))
}

// OrderUpdateSize returns the size needed to serialize an OrderUpdate event.
func OrderUpdateSize() uint64 {
	return uint64(SizeOfOrderUpdate)
}

// SerializeOrderUpdate writes an OrderUpdate to the buffer using unsafe pointer casting.
// Returns the number of bytes written.
func SerializeOrderUpdate(buf []byte, orderUpdate *event.OrderUpdate) int {
	data := (*[SizeOfOrderUpdate]byte)(unsafe.Pointer(orderUpdate))[:]
	copy(buf, data)
	return SizeOfOrderUpdate
}

// DeserializeOrderUpdate reads an OrderUpdate from buffer using unsafe pointer casting.
func DeserializeOrderUpdate(buf []byte) event.OrderUpdate {
	return *(*event.OrderUpdate)(unsafe.Pointer(&buf[0]))
}

// FillSize returns the size needed to serialize a Fill event.
func FillSize() uint64 {
	return uint64(SizeOfFill)
}

// SerializeFill writes a Fill to the buffer using unsafe pointer casting.
// Returns the number of bytes written.
func SerializeFill(buf []byte, fill *event.Fill) int {
	data := (*[SizeOfFill]byte)(unsafe.Pointer(fill))[:]
	copy(buf, data)
	return SizeOfFill
}

// DeserializeFill reads a Fill from buffer using unsafe pointer casting.
func DeserializeFill(buf []byte) event.Fill {
	return *(*event.Fill)(unsafe.Pointer(&buf[0]))
}

// DepthSnapshotSize calculates the total size needed to serialize a DepthSnapshot.
func DepthSnapshotSize(snapshot *event.DepthSnapshot) uint64 {
	asksLen := len(snapshot.Asks)
	bidsLen := len(snapshot.Bids)
	return uint64(SizeOfDepthSnapshotHeader) +
		uint64(asksLen)*uint64(SizeOfPriceLevel) +
		uint64(bidsLen)*uint64(SizeOfPriceLevel)
}

// SerializeDepthSnapshot writes a DepthSnapshot to the buffer.
// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
// Returns the number of bytes written.
func SerializeDepthSnapshot(buf []byte, snapshot *event.DepthSnapshot) int {
	asksLen := uint32(len(snapshot.Asks))
	bidsLen := uint32(len(snapshot.Bids))
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.SymbolID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.DepthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], snapshot.Timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	// Write Asks inline using unsafe
	for i := range snapshot.Asks {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Asks[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	// Write Bids inline using unsafe
	for i := range snapshot.Bids {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Bids[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	return pos
}

// WriteDepthSnapshotHeader writes only the header portion of a DepthSnapshot to the buffer.
// This is used when PriceLevels are written separately (zero-allocation path).
// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)]
func WriteDepthSnapshotHeader(buf []byte, symbolID, depthID int, timestamp uint64, asksLen, bidsLen uint32) {
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(symbolID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(depthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
}

// DeserializeDepthSnapshot reads a DepthSnapshot from buffer.
// The returned slices point directly into the buffer - no copy is made.
func DeserializeDepthSnapshot(buf []byte) event.DepthSnapshot {
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

	// Create slices pointing directly into the buffer
	var asks []event.PriceLevel
	var bids []event.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * SizeOfPriceLevel
	}

	if bidsLen > 0 {
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return event.DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
}

// DepthUpdateSize calculates the total size needed to serialize a DepthUpdate.
func DepthUpdateSize(update *event.DepthUpdate) uint64 {
	asksLen := len(update.Asks)
	bidsLen := len(update.Bids)
	return uint64(SizeOfDepthUpdateHeader) +
		uint64(asksLen)*uint64(SizeOfPriceLevel) +
		uint64(bidsLen)*uint64(SizeOfPriceLevel)
}

// SerializeDepthUpdate writes a DepthUpdate to the buffer.
// Layout: [SymbolID(8)][PreviousDepthID(8)][DepthID(8)][CurrentDepthID(8)][NextDepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
// Returns the number of bytes written.
func SerializeDepthUpdate(buf []byte, update *event.DepthUpdate) int {
	asksLen := uint32(len(update.Asks))
	bidsLen := uint32(len(update.Bids))
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.SymbolID))
	pos += 8

	// PreviousDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.PreviousDepthID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.DepthID))
	pos += 8

	// CurrentDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.CurrentDepthID))
	pos += 8

	// NextDepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.NextDepthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], update.Timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	// Write Asks inline using unsafe
	for i := range update.Asks {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&update.Asks[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	// Write Bids inline using unsafe
	for i := range update.Bids {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&update.Bids[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	return pos
}

// DeserializeDepthUpdate reads a DepthUpdate from buffer.
// The returned slices point directly into the buffer - no copy is made.
func DeserializeDepthUpdate(buf []byte) event.DepthUpdate {
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

	// Create slices pointing directly into the buffer
	var asks []event.PriceLevel
	var bids []event.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * SizeOfPriceLevel
	}

	if bidsLen > 0 {
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return event.DepthUpdate{
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

// ReqDepthSnapshotSize calculates the total size needed to serialize a ReqDepthSnapshot.
func ReqDepthSnapshotSize(snapshot *event.ReqDepthSnapshot) uint64 {
	asksLen := len(snapshot.Asks)
	bidsLen := len(snapshot.Bids)
	return uint64(SizeOfDepthSnapshotHeader) +
		uint64(asksLen)*uint64(SizeOfPriceLevel) +
		uint64(bidsLen)*uint64(SizeOfPriceLevel)
}

// SerializeReqDepthSnapshot writes a ReqDepthSnapshot to the buffer.
// Uses the same layout as DepthSnapshot.
func SerializeReqDepthSnapshot(buf []byte, snapshot *event.ReqDepthSnapshot) int {
	asksLen := uint32(len(snapshot.Asks))
	bidsLen := uint32(len(snapshot.Bids))
	pos := 0

	// SymbolID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.SymbolID))
	pos += 8

	// DepthID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.DepthID))
	pos += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], snapshot.Timestamp)
	pos += 8

	// AsksLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], asksLen)
	pos += 4

	// BidsLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], bidsLen)
	pos += 4

	// Write Asks inline using unsafe
	for i := range snapshot.Asks {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Asks[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	// Write Bids inline using unsafe
	for i := range snapshot.Bids {
		priceLevelBytes := (*[SizeOfPriceLevel]byte)(unsafe.Pointer(&snapshot.Bids[i]))[:]
		copy(buf[pos:], priceLevelBytes)
		pos += SizeOfPriceLevel
	}

	return pos
}

// DeserializeReqDepthSnapshot reads a ReqDepthSnapshot from buffer.
func DeserializeReqDepthSnapshot(buf []byte) event.ReqDepthSnapshot {
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

	// Create slices pointing directly into the buffer
	var asks []event.PriceLevel
	var bids []event.PriceLevel

	if asksLen > 0 {
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), asksLen)
		pos += int(asksLen) * SizeOfPriceLevel
	}

	if bidsLen > 0 {
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[pos])), bidsLen)
	}

	return event.ReqDepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
}

// ============================================================================
// Balance Serialization
// ============================================================================

// Size constants for balance events
const (
	SizeOfBalance = int(unsafe.Sizeof(event.Balance{}))

	// ReqBalanceSnapshot header: AccountID(8) + BalancesLen(4) + padding(4) = 16 bytes
	SizeOfReqBalanceSnapshotHeader = 16
)

// ReqBalanceSnapshotSize calculates the total size needed to serialize a ReqBalanceSnapshot.
func ReqBalanceSnapshotSize(snapshot *event.ReqBalanceSnapshot) uint64 {
	balancesLen := len(snapshot.Balances)
	return uint64(SizeOfReqBalanceSnapshotHeader) + uint64(balancesLen)*uint64(SizeOfBalance)
}

// SerializeReqBalanceSnapshot writes a ReqBalanceSnapshot to the buffer.
// Layout: [AccountID(8)][BalancesLen(4)][Padding(4)][Balances...]
// Returns the number of bytes written.
func SerializeReqBalanceSnapshot(buf []byte, snapshot *event.ReqBalanceSnapshot) int {
	balancesLen := uint32(len(snapshot.Balances))
	pos := 0

	// AccountID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(snapshot.AccountID))
	pos += 8

	// BalancesLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], balancesLen)
	pos += 4

	// Padding (4 bytes) for alignment
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Write Balances inline using unsafe
	for i := range snapshot.Balances {
		balanceBytes := (*[SizeOfBalance]byte)(unsafe.Pointer(&snapshot.Balances[i]))[:]
		copy(buf[pos:], balanceBytes)
		pos += SizeOfBalance
	}

	return pos
}

// DeserializeReqBalanceSnapshot reads a ReqBalanceSnapshot from buffer.
// The returned slice points directly into the buffer - no copy is made.
func DeserializeReqBalanceSnapshot(buf []byte) event.ReqBalanceSnapshot {
	pos := 0

	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8

	balancesLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4

	// Skip padding
	pos += 4

	// Create slice pointing directly into the buffer
	var balances []event.Balance
	if balancesLen > 0 {
		balances = unsafe.Slice((*event.Balance)(unsafe.Pointer(&buf[pos])), balancesLen)
	}

	return event.ReqBalanceSnapshot{
		AccountID: accountID,
		Balances:  balances,
	}
}
