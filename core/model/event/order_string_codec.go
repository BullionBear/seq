package event

import (
	"encoding/binary"

	"github.com/BullionBear/seq/core/model/codec"
)

// Length-prefixed binary encoding for event types that contain string fields.
// Layout encodes fixed-size int fields via LittleEndian, then each string as
// [4-byte length][UTF-8 bytes]. Fixed-prefix sizes are declared from the
// element sizes below, not hand-computed byte counts.

const (
	fieldSize     = 8 // one little-endian uint64 field
	lenPrefixSize = 4 // uint32 string length prefix
)

// ============================================================================
// OrderUnknownStatus
// Layout: [ClientOrderID(8)][OrderID(8)][AccountID(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderUnknownStatusFixedSize = 3*fieldSize + lenPrefixSize

func (o OrderUnknownStatus) GetBufferLength() int {
	return orderUnknownStatusFixedSize + len(o.Msg)
}

func (o OrderUnknownStatus) Encode(buf []byte) error {
	if len(buf) < o.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(o.ClientOrderID))
	c.PutUint64(uint64(o.OrderID))
	c.PutUint64(uint64(o.AccountID))
	c.PutUint32(uint32(len(o.Msg)))
	c.PutString(o.Msg)
	return c.Err()
}

func NewOrderUnknownStatusFromBytes(buf []byte) (OrderUnknownStatus, error) {
	if len(buf) < orderUnknownStatusFixedSize {
		return OrderUnknownStatus{}, ErrBufferTooSmall
	}
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += lenPrefixSize
	if msgLen < 0 || len(buf)-pos < msgLen {
		return OrderUnknownStatus{}, ErrInvalidBuffer
	}
	msg := string(buf[pos : pos+msgLen])
	return OrderUnknownStatus{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     accountID,
		Msg:           msg,
	}, nil
}

// ============================================================================
// OrderError
// Layout: [ClientOrderID(8)][OrderID(8)][AccountID(8)][ErrorCode(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderErrorFixedSize = 4*fieldSize + lenPrefixSize

func (o OrderError) GetBufferLength() int {
	return orderErrorFixedSize + len(o.Msg)
}

func (o OrderError) Encode(buf []byte) error {
	if len(buf) < o.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(o.ClientOrderID))
	c.PutUint64(uint64(o.OrderID))
	c.PutUint64(uint64(o.AccountID))
	c.PutUint64(uint64(o.ErrorCode))
	c.PutUint32(uint32(len(o.Msg)))
	c.PutString(o.Msg)
	return c.Err()
}

func NewOrderErrorFromBytes(buf []byte) (OrderError, error) {
	if len(buf) < orderErrorFixedSize {
		return OrderError{}, ErrBufferTooSmall
	}
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += lenPrefixSize
	if msgLen < 0 || len(buf)-pos < msgLen {
		return OrderError{}, ErrInvalidBuffer
	}
	msg := string(buf[pos : pos+msgLen])
	return OrderError{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     accountID,
		ErrorCode:     errorCode,
		Msg:           msg,
	}, nil
}

// ============================================================================
// OrderRejected
// Layout: [ClientOrderID(8)][OrderID(8)][AccountID(8)][ErrorCode(8)][UpdatedAt(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderRejectedFixedSize = 5*fieldSize + lenPrefixSize

func (o OrderRejected) GetBufferLength() int {
	return orderRejectedFixedSize + len(o.Msg)
}

func (o OrderRejected) Encode(buf []byte) error {
	if len(buf) < o.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(o.ClientOrderID))
	c.PutUint64(uint64(o.OrderID))
	c.PutUint64(uint64(o.AccountID))
	c.PutUint64(uint64(o.ErrorCode))
	c.PutUint64(o.UpdatedAt)
	c.PutUint32(uint32(len(o.Msg)))
	c.PutString(o.Msg)
	return c.Err()
}

func NewOrderRejectedFromBytes(buf []byte) (OrderRejected, error) {
	if len(buf) < orderRejectedFixedSize {
		return OrderRejected{}, ErrBufferTooSmall
	}
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	updatedAt := binary.LittleEndian.Uint64(buf[pos:])
	pos += fieldSize
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += lenPrefixSize
	if msgLen < 0 || len(buf)-pos < msgLen {
		return OrderRejected{}, ErrInvalidBuffer
	}
	msg := string(buf[pos : pos+msgLen])
	return OrderRejected{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     accountID,
		ErrorCode:     errorCode,
		UpdatedAt:     updatedAt,
		Msg:           msg,
	}, nil
}

// ============================================================================
// OrderRiskInvalid
// Layout: [ClientOrderID(8)][AccountID(8)][ErrorCode(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderRiskInvalidFixedSize = 3*fieldSize + lenPrefixSize

func (o OrderRiskInvalid) GetBufferLength() int {
	return orderRiskInvalidFixedSize + len(o.Msg)
}

func (o OrderRiskInvalid) Encode(buf []byte) error {
	if len(buf) < o.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(o.ClientOrderID))
	c.PutUint64(uint64(o.AccountID))
	c.PutUint64(uint64(o.ErrorCode))
	c.PutUint32(uint32(len(o.Msg)))
	c.PutString(o.Msg)
	return c.Err()
}

func NewOrderRiskInvalidFromBytes(buf []byte) (OrderRiskInvalid, error) {
	if len(buf) < orderRiskInvalidFixedSize {
		return OrderRiskInvalid{}, ErrBufferTooSmall
	}
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += fieldSize
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += lenPrefixSize
	if msgLen < 0 || len(buf)-pos < msgLen {
		return OrderRiskInvalid{}, ErrInvalidBuffer
	}
	msg := string(buf[pos : pos+msgLen])
	return OrderRiskInvalid{
		ClientOrderID: clientOrderID,
		AccountID:     accountID,
		ErrorCode:     errorCode,
		Msg:           msg,
	}, nil
}
