package model

import "time"

type PriceLevel struct {
	Price    float64
	Quantity float64
}

type DepthUpdate struct {
	SymbolID        int
	PreviousDepthID int
	DepthID         int
	NextDepthID     int
	Timestamp       time.Time
	Asks            []PriceLevel
	Bids            []PriceLevel
}

type DepthSnapshot struct {
	SymbolID  int
	DepthID   int
	Timestamp time.Time
	Asks      []PriceLevel
	Bids      []PriceLevel
}
