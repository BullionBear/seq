package model

import "time"

type Tick struct {
	SymbolID  int
	Price     float64
	Side      Side
	Quantity  float64
	Timestamp time.Time
}
