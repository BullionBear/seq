package model

import "time"

type OrderUpdate struct {
	ClientOrderID int
	Status        Status
	ExecutedQty   float64
	UpdatedAt     time.Time
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
