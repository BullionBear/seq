package event

import (
	"github.com/BullionBear/seq/core/model"
)

type OrderUpdate struct {
	OrderID     int
	Status      model.Status
	ExecutedQty float64
	UpdatedAt   uint64
}

type Fill struct {
	OrderID     int
	FillID      int
	FilledQty   float64
	FilledPrice float64
	FeeCcyID    int
	FeeQty      float64
	FilledAt    uint64
}
