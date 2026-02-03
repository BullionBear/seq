package event

type OrderUnknownStatus struct {
	ClientOrderID int
	OrderID       int
	Msg           string
}

type OrderError struct {
	ClientOrderID int
	OrderID       int
	ErrorCode     int
	Msg           string
}

type OrderAccepted struct {
	ClientOrderID int
	OrderID       int
	CreatedAt     uint64
}

type OrderPartiallyFilled struct {
	ClientOrderID int
	OrderID       int
	ExecutedQty   float64
	UpdatedAt     uint64
}

type OrderFilled struct {
	ClientOrderID int
	OrderID       int
	ExecutedQty   float64
	UpdatedAt     uint64
}

type OrderCanceled struct {
	ClientOrderID int
	OrderID       int
	UpdatedAt     uint64
}

type OrderRejected struct {
	ClientOrderID int
	OrderID       int
	ErrorCode     int
	Msg           string
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
