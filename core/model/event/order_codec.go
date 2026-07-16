package event

import "unsafe"

// Size constants for order event types (fixed-size structs only)
const (
	sizeOfOrderNew             = int(unsafe.Sizeof(OrderNew{}))
	sizeOfOrderAccepted        = int(unsafe.Sizeof(OrderAccepted{}))
	sizeOfOrderPartiallyFilled = int(unsafe.Sizeof(OrderPartiallyFilled{}))
	sizeOfOrderFilled          = int(unsafe.Sizeof(OrderFilled{}))
	sizeOfOrderCanceled        = int(unsafe.Sizeof(OrderCanceled{}))
	sizeOfExecution            = int(unsafe.Sizeof(Execution{}))
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

func NewOrderNewFromBytes(buf []byte) (OrderNew, error) {
	var v OrderNew
	if len(buf) < sizeOfOrderNew {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfOrderNew]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewOrderAcceptedFromBytes(buf []byte) (OrderAccepted, error) {
	var v OrderAccepted
	if len(buf) < sizeOfOrderAccepted {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfOrderAccepted]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewOrderPartiallyFilledFromBytes(buf []byte) (OrderPartiallyFilled, error) {
	var v OrderPartiallyFilled
	if len(buf) < sizeOfOrderPartiallyFilled {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfOrderPartiallyFilled]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewOrderFilledFromBytes(buf []byte) (OrderFilled, error) {
	var v OrderFilled
	if len(buf) < sizeOfOrderFilled {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfOrderFilled]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
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

func NewOrderCanceledFromBytes(buf []byte) (OrderCanceled, error) {
	var v OrderCanceled
	if len(buf) < sizeOfOrderCanceled {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfOrderCanceled]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
}

// ============================================================================
// Execution
// ============================================================================

func (e Execution) GetBufferLength() int { return sizeOfExecution }

func (e Execution) Encode(buf []byte) error {
	if len(buf) < sizeOfExecution {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfExecution]byte)(unsafe.Pointer(&e))[:]
	copy(buf, data)
	return nil
}

func NewExecutionFromBytes(buf []byte) (Execution, error) {
	var v Execution
	if len(buf) < sizeOfExecution {
		return v, ErrBufferTooSmall
	}
	copy((*[sizeOfExecution]byte)(unsafe.Pointer(&v))[:], buf)
	return v, nil
}
