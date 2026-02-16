package event

import "encoding/binary"

// Length-prefixed binary encoding for event types that contain string fields.
// Layout encodes fixed-size int fields via LittleEndian, then each string as
// [4-byte length][UTF-8 bytes].

// ============================================================================
// OrderUnknownStatus
// Layout: [ClientOrderID(8)][OrderID(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderUnknownStatusFixedSize = 8 + 8 + 4 // 20 bytes

func (o OrderUnknownStatus) GetBufferLength() int {
	return orderUnknownStatusFixedSize + len(o.Msg)
}

func (o OrderUnknownStatus) Encode(buf []byte) error {
	needed := o.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ClientOrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.OrderID))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(o.Msg)))
	pos += 4
	copy(buf[pos:], o.Msg)
	return nil
}

func NewOrderUnknownStatusFromBytes(buf []byte) OrderUnknownStatus {
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4
	msg := string(buf[pos : pos+msgLen])
	return OrderUnknownStatus{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		Msg:           msg,
	}
}

// ============================================================================
// OrderError
// Layout: [ClientOrderID(8)][OrderID(8)][ErrorCode(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderErrorFixedSize = 8 + 8 + 8 + 4 // 28 bytes

func (o OrderError) GetBufferLength() int {
	return orderErrorFixedSize + len(o.Msg)
}

func (o OrderError) Encode(buf []byte) error {
	needed := o.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ClientOrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.OrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ErrorCode))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(o.Msg)))
	pos += 4
	copy(buf[pos:], o.Msg)
	return nil
}

func NewOrderErrorFromBytes(buf []byte) OrderError {
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4
	msg := string(buf[pos : pos+msgLen])
	return OrderError{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		ErrorCode:     errorCode,
		Msg:           msg,
	}
}

// ============================================================================
// OrderRejected
// Layout: [ClientOrderID(8)][OrderID(8)][ErrorCode(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderRejectedFixedSize = 8 + 8 + 8 + 4 // 28 bytes

func (o OrderRejected) GetBufferLength() int {
	return orderRejectedFixedSize + len(o.Msg)
}

func (o OrderRejected) Encode(buf []byte) error {
	needed := o.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ClientOrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.OrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ErrorCode))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(o.Msg)))
	pos += 4
	copy(buf[pos:], o.Msg)
	return nil
}

func NewOrderRejectedFromBytes(buf []byte) OrderRejected {
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	orderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4
	msg := string(buf[pos : pos+msgLen])
	return OrderRejected{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		ErrorCode:     errorCode,
		Msg:           msg,
	}
}

// ============================================================================
// OrderRiskInvalid
// Layout: [ClientOrderID(8)][ErrorCode(8)][MsgLen(4)][Msg(...)]
// ============================================================================

const orderRiskInvalidFixedSize = 8 + 8 + 4 // 20 bytes

func (o OrderRiskInvalid) GetBufferLength() int {
	return orderRiskInvalidFixedSize + len(o.Msg)
}

func (o OrderRiskInvalid) Encode(buf []byte) error {
	needed := o.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ClientOrderID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(o.ErrorCode))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(o.Msg)))
	pos += 4
	copy(buf[pos:], o.Msg)
	return nil
}

func NewOrderRiskInvalidFromBytes(buf []byte) OrderRiskInvalid {
	pos := 0
	clientOrderID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	errorCode := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	msgLen := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4
	msg := string(buf[pos : pos+msgLen])
	return OrderRiskInvalid{
		ClientOrderID: clientOrderID,
		ErrorCode:     errorCode,
		Msg:           msg,
	}
}
