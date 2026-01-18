package ems

import (
	"github.com/BullionBear/seq/internal/srv"
)

type OrderType int

const (
	TypeMarket OrderType = iota
	TypeLimit
)

type TimeInForce int

const (
	TimeInForceGTC TimeInForce = iota
	TimeInForceIOC
	TimeInForceFOK
	TimeInForcePO
)

type Status int

const (
	StatusUninitialized Status = iota
	StatusInitialized
	StatusInFlight
	StatusAccepted
	StatusPartiallyFilled
	StatusFilled
	StatusCanceled
	StatusRejected
)

type Order struct {
	StrategyID    int
	AcctID        int
	ClientOrderID int
	SymbolID      int
	Side          srv.Side
	Type          OrderType
	TimeInForce   TimeInForce
	Price         float64
	Quantity      float64
	ExecutedQty   float64
	Status        Status
	CreatedAt     uint64
	UpdatedAt     uint64
}

type OrderUpdate struct {
	ClientOrderID     int
	BeforeStatus      Status
	AfterStatus       Status
	BeforeExecutedQty float64
	AfterExecutedQty  float64
	UpdatedAt         uint64
}

func (o *OrderUpdate) Reset() {
	o.BeforeStatus = StatusUninitialized
	o.AfterStatus = StatusUninitialized
	o.BeforeExecutedQty = 0
	o.AfterExecutedQty = 0
	o.UpdatedAt = 0
}

type Fill struct {
	ClientOrderID int
	FillID        int
	FilledQty     float64
	FilledPrice   float64
	FeeCcyID      int
	FeeQty        float64
	FilledAt      uint64
}

func (f *Fill) Reset() {
	f.ClientOrderID = 0
	f.FillID = 0
	f.FilledQty = 0
	f.FilledPrice = 0
	f.FeeQty = 0
	f.FeeCcyID = 0
	f.FilledAt = 0
}
