package event

type OrderUnknownStatus struct {
	ClientOrderID int
	OrderID       int
	Msg           string
}

func (o OrderUnknownStatus) Topic() Topic {
	return TopicEventOrderUnknownStatus
}

type OrderError struct {
	ClientOrderID int
	OrderID       int
	ErrorCode     int
	Msg           string
}

func (o OrderError) Topic() Topic {
	return TopicEventOrderError
}

type OrderRiskInvalid struct {
	ClientOrderID int
	ErrorCode     int
	Msg           string
}

func (o OrderRiskInvalid) Topic() Topic {
	return TopicEventOrderRiskInvalid
}

type OrderNew struct {
	ClientOrderID int
	OrderID       int
	CreatedAt     uint64
}

func (o OrderNew) Topic() Topic {
	return TopicEventOrderNew
}

type OrderAccepted struct {
	ClientOrderID int
	OrderID       int
	CreatedAt     uint64
}

func (o OrderAccepted) Topic() Topic {
	return TopicEventOrderAccepted
}

type OrderPartiallyFilled struct {
	ClientOrderID int
	OrderID       int
	ExecutedQty   float64
	UpdatedAt     uint64
}

func (o OrderPartiallyFilled) Topic() Topic {
	return TopicEventPartialFill
}

type OrderFilled struct {
	ClientOrderID int
	OrderID       int
	ExecutedQty   float64
	UpdatedAt     uint64
}

func (o OrderFilled) Topic() Topic {
	return TopicEventFill
}

type OrderCanceled struct {
	ClientOrderID int
	OrderID       int
	UpdatedAt     uint64
}

func (o OrderCanceled) Topic() Topic {
	return TopicEventOrderCanceled
}

type OrderRejected struct {
	ClientOrderID int
	OrderID       int
	ErrorCode     int
	Msg           string
}

func (o OrderRejected) Topic() Topic {
	return TopicEventOrderRejected
}

type Fill struct {
	ClientOrderID int
	OrderID       int
	FillID        int
	FilledQty     float64
	FilledPrice   float64
	FeeCcyID      int
	FeeQty        float64
	FilledAt      uint64
}

func (f Fill) Topic() Topic {
	return TopicEventFill
}
