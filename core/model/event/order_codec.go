package event

import "unsafe"

// Size constants for order event types (fixed-size structs only)
const (
	sizeOfOrderNew             = int(unsafe.Sizeof(OrderNew{}))
	sizeOfOrderAccepted        = int(unsafe.Sizeof(OrderAccepted{}))
	sizeOfOrderPartiallyFilled = int(unsafe.Sizeof(OrderPartiallyFilled{}))
	sizeOfOrderFilled          = int(unsafe.Sizeof(OrderFilled{}))
	sizeOfOrderCanceled        = int(unsafe.Sizeof(OrderCanceled{}))
	sizeOfFill                 = int(unsafe.Sizeof(Fill{}))
)

// ============================================================================
// OrderNew
// ============================================================================

func (o OrderNew) GetBufferLength() int { return sizeOfOrderNew }

func (o OrderNew) Encode(buf []byte) error {
	if len(buf) < sizeOfOrderNew {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfOrderNew]byte)(unsafe.Pointer(&o))[:]
	copy(buf, data)
	return nil
}

func NewOrderNewFromBytes(buf []byte) OrderNew {
	return *(*OrderNew)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// OrderAccepted
// ============================================================================

func (o OrderAccepted) GetBufferLength() int { return sizeOfOrderAccepted }

func (o OrderAccepted) Encode(buf []byte) error {
	if len(buf) < sizeOfOrderAccepted {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfOrderAccepted]byte)(unsafe.Pointer(&o))[:]
	copy(buf, data)
	return nil
}

func NewOrderAcceptedFromBytes(buf []byte) OrderAccepted {
	return *(*OrderAccepted)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// OrderPartiallyFilled
// ============================================================================

func (o OrderPartiallyFilled) GetBufferLength() int { return sizeOfOrderPartiallyFilled }

func (o OrderPartiallyFilled) Encode(buf []byte) error {
	if len(buf) < sizeOfOrderPartiallyFilled {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfOrderPartiallyFilled]byte)(unsafe.Pointer(&o))[:]
	copy(buf, data)
	return nil
}

func NewOrderPartiallyFilledFromBytes(buf []byte) OrderPartiallyFilled {
	return *(*OrderPartiallyFilled)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// OrderFilled
// ============================================================================

func (o OrderFilled) GetBufferLength() int { return sizeOfOrderFilled }

func (o OrderFilled) Encode(buf []byte) error {
	if len(buf) < sizeOfOrderFilled {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfOrderFilled]byte)(unsafe.Pointer(&o))[:]
	copy(buf, data)
	return nil
}

func NewOrderFilledFromBytes(buf []byte) OrderFilled {
	return *(*OrderFilled)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// OrderCanceled
// ============================================================================

func (o OrderCanceled) GetBufferLength() int { return sizeOfOrderCanceled }

func (o OrderCanceled) Encode(buf []byte) error {
	if len(buf) < sizeOfOrderCanceled {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfOrderCanceled]byte)(unsafe.Pointer(&o))[:]
	copy(buf, data)
	return nil
}

func NewOrderCanceledFromBytes(buf []byte) OrderCanceled {
	return *(*OrderCanceled)(unsafe.Pointer(&buf[0]))
}

// ============================================================================
// Fill
// ============================================================================

func (f Fill) GetBufferLength() int { return sizeOfFill }

func (f Fill) Encode(buf []byte) error {
	if len(buf) < sizeOfFill {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfFill]byte)(unsafe.Pointer(&f))[:]
	copy(buf, data)
	return nil
}

func NewFillFromBytes(buf []byte) Fill {
	return *(*Fill)(unsafe.Pointer(&buf[0]))
}
