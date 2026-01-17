package binance

const (
	// MaxDepthLevels is the maximum number of price levels per side in an order book
	// 5000 is the maximum supported by Binance API
	MaxDepthLevels = 5000
)

type PriceLevel struct {
	Price    float64
	Quantity float64
}

// DepthResponse represents the order book depth response from Binance
// Uses embedded arrays with length tracking for zero-allocation design
type DepthResponse struct {
	LastUpdateID int64
	bidsArray    [MaxDepthLevels]PriceLevel // Pre-allocated array for bids
	bidsLen      int                        // Actual length of bids
	asksArray    [MaxDepthLevels]PriceLevel // Pre-allocated array for asks
	asksLen      int                        // Actual length of asks
}

// Bids returns a pointer to the bid at index i (zero-allocation, returns pointer to embedded array)
func (d *DepthResponse) Bids(i int) *PriceLevel {
	if i < 0 || i >= d.bidsLen {
		return nil
	}
	return &d.bidsArray[i]
}

// Asks returns a pointer to the ask at index i (zero-allocation, returns pointer to embedded array)
func (d *DepthResponse) Asks(i int) *PriceLevel {
	if i < 0 || i >= d.asksLen {
		return nil
	}
	return &d.asksArray[i]
}

// BidsLen returns the number of bids
func (d *DepthResponse) BidsLen() int {
	return d.bidsLen
}

// AsksLen returns the number of asks
func (d *DepthResponse) AsksLen() int {
	return d.asksLen
}

// Reset clears all fields and resets length counters (arrays stay allocated)
func (d *DepthResponse) Reset() {
	d.LastUpdateID = 0
	d.bidsLen = 0
	d.asksLen = 0
	// Arrays remain allocated, just reset lengths
}

// DepthParams represents parameters for the depth endpoint
type DepthParams struct {
	Symbol string
	Limit  int
}

// AuthorizationParams represents parameters for the authorization endpoint
type AuthenticationParams struct {
	APIKey    string
	SecretKey string
}

type CreateOrderRequest struct {
	Symbol   string
	Side     string
	Type     string
	Price    float64
	Quantity float64
}
