package mds

import (
	"time"

	"github.com/BullionBear/seq/internal/srv"
)

type Tick struct {
	TickID    int
	SymbolID  int
	Side      srv.Side
	Price     float64
	Quantity  float64
	Timestamp time.Time
}

func (t *Tick) Reset() {
	t.TickID = 0
	t.SymbolID = 0
	t.Side = srv.SideUnknown
	t.Price = 0
	t.Quantity = 0
	t.Timestamp = time.Time{}
}

type PriceLevel struct {
	Price    float64
	Quantity float64
	isZero   bool
}

const (
	// MaxDepthLevels is the maximum number of price levels per side in an order book
	// 256 levels covers 99.9% of real-world order books (typically 20-100 levels)
	MaxDepthLevels = 256
)

type DepthUpdate struct {
	SymbolID        int
	DepthID         int
	PreviousDepthID int
	asksArray       []PriceLevel // Embedded array (zero allocation)
	AsksLen         int                        // Actual length of asks
	bidsArray       []PriceLevel // Embedded array (zero allocation)
	BidsLen         int                        // Actual length of bids
	Timestamp       time.Time
}

// Asks returns a slice view of the asks array (no allocation, just a slice header)
func (d *DepthUpdate) Asks() []PriceLevel {
	return d.asksArray[:d.AsksLen]
}

// Bids returns a slice view of the bids array (no allocation, just a slice header)
func (d *DepthUpdate) Bids() []PriceLevel {
	return d.bidsArray[:d.BidsLen]
}

// SetAsks sets asks from a slice (copies into embedded array)
func (d *DepthUpdate) SetAsks(asks []PriceLevel) {
	if len(asks) > MaxDepthLevels {
		asks = asks[:MaxDepthLevels] // Truncate if too large
	}
	d.AsksLen = copy(d.asksArray[:], asks)
}

// SetBids sets bids from a slice (copies into embedded array)
func (d *DepthUpdate) SetBids(bids []PriceLevel) {
	if len(bids) > MaxDepthLevels {
		bids = bids[:MaxDepthLevels] // Truncate if too large
	}
	d.BidsLen = copy(d.bidsArray[:], bids)
}

// Reset clears all fields and resets length counters (arrays stay allocated)
func (d *DepthUpdate) Reset() {
	d.SymbolID = 0
	d.DepthID = 0
	d.PreviousDepthID = 0
	d.AsksLen = 0
	d.BidsLen = 0
	d.Timestamp = time.Time{}
	// Note: We don't zero the arrays - they'll be overwritten on next use
	// This avoids unnecessary work in the hot path
}
