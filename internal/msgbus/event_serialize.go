package msgbus

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/event"
)

// Size constants for fixed-size event types
const (
	SizeOfTick       = int(unsafe.Sizeof(event.Tick{}))
	SizeOfPriceLevel = int(unsafe.Sizeof(event.PriceLevel{}))

	// State event sizes
	SizeOfReadyEvent    = int(unsafe.Sizeof(event.ReadyEvent{}))
	SizeOfStopEvent     = int(unsafe.Sizeof(event.StopEvent{}))
	SizeOfFinishedEvent = int(unsafe.Sizeof(event.FinishedEvent{}))
	SizeOfAbnormalEvent = int(unsafe.Sizeof(event.AbnormalEvent{}))

	// Order event sizes
	SizeOfOrderUnknownStatus   = int(unsafe.Sizeof(event.OrderUnknownStatus{}))
	SizeOfOrderError           = int(unsafe.Sizeof(event.OrderError{}))
	SizeOfOrderAccepted        = int(unsafe.Sizeof(event.OrderAccepted{}))
	SizeOfOrderPartiallyFilled = int(unsafe.Sizeof(event.OrderPartiallyFilled{}))
	SizeOfOrderFilled          = int(unsafe.Sizeof(event.OrderFilled{}))
	SizeOfOrderCanceled        = int(unsafe.Sizeof(event.OrderCanceled{}))
	SizeOfOrderRejected        = int(unsafe.Sizeof(event.OrderRejected{}))
	SizeOfFill                 = int(unsafe.Sizeof(event.Fill{}))

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

// ============================================================================
// Order Event Serialization
// ============================================================================

// OrderAcceptedSize returns the size needed to serialize an OrderAccepted event.
func OrderAcceptedSize() uint64 {
	return uint64(SizeOfOrderAccepted)
}

// SerializeOrderAccepted writes an OrderAccepted to the buffer.
func SerializeOrderAccepted(buf []byte, e *event.OrderAccepted) int {
	data := (*[SizeOfOrderAccepted]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderAccepted
}

// DeserializeOrderAccepted reads an OrderAccepted from buffer.
func DeserializeOrderAccepted(buf []byte) event.OrderAccepted {
	return *(*event.OrderAccepted)(unsafe.Pointer(&buf[0]))
}

// OrderPartiallyFilledSize returns the size needed to serialize an OrderPartiallyFilled event.
func OrderPartiallyFilledSize() uint64 {
	return uint64(SizeOfOrderPartiallyFilled)
}

// SerializeOrderPartiallyFilled writes an OrderPartiallyFilled to the buffer.
func SerializeOrderPartiallyFilled(buf []byte, e *event.OrderPartiallyFilled) int {
	data := (*[SizeOfOrderPartiallyFilled]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderPartiallyFilled
}

// DeserializeOrderPartiallyFilled reads an OrderPartiallyFilled from buffer.
func DeserializeOrderPartiallyFilled(buf []byte) event.OrderPartiallyFilled {
	return *(*event.OrderPartiallyFilled)(unsafe.Pointer(&buf[0]))
}

// OrderFilledSize returns the size needed to serialize an OrderFilled event.
func OrderFilledSize() uint64 {
	return uint64(SizeOfOrderFilled)
}

// SerializeOrderFilled writes an OrderFilled to the buffer.
func SerializeOrderFilled(buf []byte, e *event.OrderFilled) int {
	data := (*[SizeOfOrderFilled]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderFilled
}

// DeserializeOrderFilled reads an OrderFilled from buffer.
func DeserializeOrderFilled(buf []byte) event.OrderFilled {
	return *(*event.OrderFilled)(unsafe.Pointer(&buf[0]))
}

// OrderCanceledSize returns the size needed to serialize an OrderCanceled event.
func OrderCanceledSize() uint64 {
	return uint64(SizeOfOrderCanceled)
}

// SerializeOrderCanceled writes an OrderCanceled to the buffer.
func SerializeOrderCanceled(buf []byte, e *event.OrderCanceled) int {
	data := (*[SizeOfOrderCanceled]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderCanceled
}

// DeserializeOrderCanceled reads an OrderCanceled from buffer.
func DeserializeOrderCanceled(buf []byte) event.OrderCanceled {
	return *(*event.OrderCanceled)(unsafe.Pointer(&buf[0]))
}

// OrderRejectedSize returns the size needed to serialize an OrderRejected event.
func OrderRejectedSize() uint64 {
	return uint64(SizeOfOrderRejected)
}

// SerializeOrderRejected writes an OrderRejected to the buffer.
func SerializeOrderRejected(buf []byte, e *event.OrderRejected) int {
	data := (*[SizeOfOrderRejected]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderRejected
}

// DeserializeOrderRejected reads an OrderRejected from buffer.
func DeserializeOrderRejected(buf []byte) event.OrderRejected {
	return *(*event.OrderRejected)(unsafe.Pointer(&buf[0]))
}

// OrderUnknownStatusSize returns the size needed to serialize an OrderUnknownStatus event.
func OrderUnknownStatusSize() uint64 {
	return uint64(SizeOfOrderUnknownStatus)
}

// SerializeOrderUnknownStatus writes an OrderUnknownStatus to the buffer.
func SerializeOrderUnknownStatus(buf []byte, e *event.OrderUnknownStatus) int {
	data := (*[SizeOfOrderUnknownStatus]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderUnknownStatus
}

// DeserializeOrderUnknownStatus reads an OrderUnknownStatus from buffer.
func DeserializeOrderUnknownStatus(buf []byte) event.OrderUnknownStatus {
	return *(*event.OrderUnknownStatus)(unsafe.Pointer(&buf[0]))
}

// OrderErrorSize returns the size needed to serialize an OrderError event.
func OrderErrorSize() uint64 {
	return uint64(SizeOfOrderError)
}

// SerializeOrderError writes an OrderError to the buffer.
func SerializeOrderError(buf []byte, e *event.OrderError) int {
	data := (*[SizeOfOrderError]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfOrderError
}

// DeserializeOrderError reads an OrderError from buffer.
func DeserializeOrderError(buf []byte) event.OrderError {
	return *(*event.OrderError)(unsafe.Pointer(&buf[0]))
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

// ReqBalanceSnapshotSize calculates the total size needed to serialize a RespBalanceSnapshot.
func RespBalanceSnapshotSize(snapshot *event.RespBalanceSnapshot) uint64 {
	balancesLen := len(snapshot.Balances)
	return uint64(SizeOfReqBalanceSnapshotHeader) + uint64(balancesLen)*uint64(SizeOfBalance)
}

// SerializeRespBalanceSnapshot writes a RespBalanceSnapshot to the buffer.
// Layout: [AccountID(8)][BalancesLen(4)][Padding(4)][Balances...]
// Returns the number of bytes written.
func SerializeRespBalanceSnapshot(buf []byte, snapshot *event.RespBalanceSnapshot) int {
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

// DeserializeRespBalanceSnapshot reads a RespBalanceSnapshot from buffer.
// The returned slice points directly into the buffer - no copy is made.
func DeserializeRespBalanceSnapshot(buf []byte) event.RespBalanceSnapshot {
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

	return event.RespBalanceSnapshot{
		AccountID: accountID,
		Balances:  balances,
	}
}

// ============================================================================
// BalanceUpdate Serialization
// ============================================================================

const (
	// BalanceUpdate header: AccountID(8) + UpdatedAt(8) + BalancesLen(4) + padding(4) = 24 bytes
	SizeOfBalanceUpdateHeader = 24
)

// BalanceUpdateSize calculates the total size needed to serialize a BalanceUpdate.
func BalanceUpdateSize(update *event.BalanceUpdate) uint64 {
	balancesLen := len(update.Balances)
	return uint64(SizeOfBalanceUpdateHeader) + uint64(balancesLen)*uint64(SizeOfBalance)
}

// SerializeBalanceUpdate writes a BalanceUpdate to the buffer.
// Layout: [AccountID(8)][UpdatedAt(8)][BalancesLen(4)][Padding(4)][Balances...]
// Returns the number of bytes written.
func SerializeBalanceUpdate(buf []byte, update *event.BalanceUpdate) int {
	balancesLen := uint32(len(update.Balances))
	pos := 0

	// AccountID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(update.AccountID))
	pos += 8

	// UpdatedAt (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], update.UpdatedAt)
	pos += 8

	// BalancesLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], balancesLen)
	pos += 4

	// Padding (4 bytes) for alignment
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Write Balances inline using unsafe
	for i := range update.Balances {
		balanceBytes := (*[SizeOfBalance]byte)(unsafe.Pointer(&update.Balances[i]))[:]
		copy(buf[pos:], balanceBytes)
		pos += SizeOfBalance
	}

	return pos
}

// DeserializeBalanceUpdate reads a BalanceUpdate from buffer.
// The returned slice points directly into the buffer - no copy is made.
func DeserializeBalanceUpdate(buf []byte) event.BalanceUpdate {
	pos := 0

	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8

	updatedAt := binary.LittleEndian.Uint64(buf[pos:])
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

	return event.BalanceUpdate{
		AccountID: accountID,
		Balances:  balances,
		UpdatedAt: updatedAt,
	}
}

// ============================================================================
// State Event Serialization
// ============================================================================

// ReadyEventSize returns the size needed to serialize a ReadyEvent.
func ReadyEventSize() uint64 {
	return uint64(SizeOfReadyEvent)
}

// SerializeReadyEvent writes a ReadyEvent to the buffer.
func SerializeReadyEvent(buf []byte, e *event.ReadyEvent) int {
	data := (*[SizeOfReadyEvent]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfReadyEvent
}

// DeserializeReadyEvent reads a ReadyEvent from buffer.
func DeserializeReadyEvent(buf []byte) event.ReadyEvent {
	return *(*event.ReadyEvent)(unsafe.Pointer(&buf[0]))
}

// StopEventSize returns the size needed to serialize a StopEvent.
func StopEventSize() uint64 {
	return uint64(SizeOfStopEvent)
}

// SerializeStopEvent writes a StopEvent to the buffer.
func SerializeStopEvent(buf []byte, e *event.StopEvent) int {
	data := (*[SizeOfStopEvent]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfStopEvent
}

// DeserializeStopEvent reads a StopEvent from buffer.
func DeserializeStopEvent(buf []byte) event.StopEvent {
	return *(*event.StopEvent)(unsafe.Pointer(&buf[0]))
}

// FinishedEventSize returns the size needed to serialize a FinishedEvent.
func FinishedEventSize() uint64 {
	return uint64(SizeOfFinishedEvent)
}

// SerializeFinishedEvent writes a FinishedEvent to the buffer.
func SerializeFinishedEvent(buf []byte, e *event.FinishedEvent) int {
	data := (*[SizeOfFinishedEvent]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfFinishedEvent
}

// DeserializeFinishedEvent reads a FinishedEvent from buffer.
func DeserializeFinishedEvent(buf []byte) event.FinishedEvent {
	return *(*event.FinishedEvent)(unsafe.Pointer(&buf[0]))
}

// AbnormalEventSize returns the size needed to serialize an AbnormalEvent.
func AbnormalEventSize() uint64 {
	return uint64(SizeOfAbnormalEvent)
}

// SerializeAbnormalEvent writes an AbnormalEvent to the buffer.
func SerializeAbnormalEvent(buf []byte, e *event.AbnormalEvent) int {
	data := (*[SizeOfAbnormalEvent]byte)(unsafe.Pointer(e))[:]
	copy(buf, data)
	return SizeOfAbnormalEvent
}

// DeserializeAbnormalEvent reads an AbnormalEvent from buffer.
func DeserializeAbnormalEvent(buf []byte) event.AbnormalEvent {
	return *(*event.AbnormalEvent)(unsafe.Pointer(&buf[0]))
}
