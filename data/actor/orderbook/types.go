package orderbook

import "github.com/BullionBear/seq/core/model/common"

// DepthUpdateBuffer holds a buffered depth update during sync.
// For Binance depth updates:
//   - PreviousDepthID = U - 1 (one before first update ID)
//   - FirstDepthID = U (first update ID in event)
//   - FinalDepthID = u (final update ID in event)
type DepthUpdateBuffer struct {
	PreviousDepthID int
	FirstDepthID    int
	FinalDepthID    int
	Timestamp       uint64
	Bids            []common.PriceLevel
	Asks            []common.PriceLevel
}
