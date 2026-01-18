package event

import "github.com/BullionBear/seq/core/model"

type Tick struct {
	SymbolID  int
	Timestamp uint64
	Side      model.Side
	Price     float64
	Qty       float64
}
