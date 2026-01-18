package event

import "github.com/BullionBear/seq/core/model/common"

type Tick struct {
	SymbolID  int
	Timestamp uint64
	Side      common.Side
	Price     float64
	Qty       float64
}
