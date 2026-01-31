package event

type PriceLevel struct {
	Price    float64
	Quantity float64
}

type DepthUpdate struct {
	SymbolID        int
	PreviousDepthID int
	DepthID         int
	CurrentDepthID  int
	NextDepthID     int
	Timestamp       uint64
	Asks            []PriceLevel
	Bids            []PriceLevel
}

type DepthSnapshot struct {
	SymbolID  int
	DepthID   int
	Timestamp uint64
	Asks      []PriceLevel
	Bids      []PriceLevel
}

type ReqDepthSnapshot struct {
	SymbolID  int
	DepthID   int
	Timestamp uint64
	AskLength int          // number of ask price levels
	BidLength int          // number of bid price levels
	Asks      []PriceLevel // points into arena buffer
	Bids      []PriceLevel // points into arena buffer
}
