package ems

import (
	"time"
)

type Side int

const (
	SideBuy Side = iota
	SideSell
)

type Type int

const (
	TypeMarket Type = iota
	TypeLimit
)

type Status int

const (
	StatusUninitialized Status = iota
	StatusInitialized
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
	Side          Side
	Type          Type
	Price         float64
	Quantity      float64
	ExecutedQty   float64
	Status        Status
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OrderUpdate struct {
	ClientOrderID     int
	BeforeStatus      Status
	AfterStatus       Status
	BeforeExecutedQty float64
	AfterExecutedQty  float64
	UpdatedAt         time.Time
}

func (o *OrderUpdate) Reset() {
	o.BeforeStatus = StatusUninitialized
	o.AfterStatus = StatusUninitialized
	o.BeforeExecutedQty = 0
	o.AfterExecutedQty = 0
	o.UpdatedAt = time.Time{}
}

type OrderFill struct {
	ClientOrderID int
	FillID        int
	FilledQty     float64
	FilledPrice   float64
	FeeCcyID      int
	FeeQty        float64
	FilledAt      time.Time
}

func (f *OrderFill) Reset() {
	f.ClientOrderID = 0
	f.FillID = 0
	f.FilledQty = 0
	f.FilledPrice = 0
	f.FeeQty = 0
	f.FeeCcyID = 0
	f.FilledAt = time.Time{}
}
