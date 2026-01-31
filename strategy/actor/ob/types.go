package ob

// BookState represents the synchronization state of an orderbook
type BookState int

const (
	StateWaitForSnapshot BookState = iota
	StateUpdating
	StateReady
)

// String returns the string representation of BookState
func (s BookState) String() string {
	switch s {
	case StateWaitForSnapshot:
		return "WaitForSnapshot"
	case StateUpdating:
		return "Updating"
	case StateReady:
		return "Ready"
	default:
		return "Unknown"
	}
}

// PriceLevel is the orderbook's own price level type (separate from event.PriceLevel)
type PriceLevel struct {
	PriceTick int64   // price as integer tick
	Quantity  float64 // quantity at this price level
}

// DepthUpdateBuffer holds a buffered depth update during sync
// For Binance depth updates:
//   - PreviousDepthID = U - 1 (one before first update ID)
//   - FirstDepthID = U (first update ID in event)
//   - FinalDepthID = u (final update ID in event)
type DepthUpdateBuffer struct {
	PreviousDepthID int
	FirstDepthID    int
	FinalDepthID    int
	Timestamp       uint64
	Asks            []PriceLevel
	Bids            []PriceLevel
}
