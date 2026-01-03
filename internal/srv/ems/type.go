package ems

import (
	"time"

	"github.com/shopspring/decimal"
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
	Price         decimal.Decimal
	Quantity      decimal.Decimal
	ExecutedQty   decimal.Decimal
	Status        Status
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OrderUpdate struct {
	ClientOrderID     int
	BeforeStatus      Status
	AfterStatus       Status
	BeforeExecutedQty decimal.Decimal
	AfterExecutedQty  decimal.Decimal
	UpdatedAt         time.Time
}

func (o *OrderUpdate) Reset() {
	o.BeforeStatus = StatusUninitialized
	o.AfterStatus = StatusUninitialized
	o.BeforeExecutedQty = decimal.Zero
	o.AfterExecutedQty = decimal.Zero
	o.UpdatedAt = time.Time{}
}

type OrderFill struct {
	ClientOrderID int
	FillID        int
	FilledQty     decimal.Decimal
	FilledPrice   decimal.Decimal
	FeeCcyID      int
	FeeQty        decimal.Decimal
	FilledAt      time.Time
}

func (f *OrderFill) Reset() {
	f.ClientOrderID = 0
	f.FillID = 0
	f.FilledQty = decimal.Zero
	f.FilledPrice = decimal.Zero
	f.FeeQty = decimal.Zero
	f.FeeCcyID = 0
	f.FilledAt = time.Time{}
}
