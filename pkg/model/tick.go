package model

import "time"

type Tick struct {
	SymbolID  int
	Price     float64
	Quantity  float64
	Timestamp time.Time
}
