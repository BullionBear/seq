package bybit

// Category represents Bybit product types
// Bybit V5 API unifies all products into one endpoint with category parameter
type Category string

const (
	CategorySpot    Category = "spot"
	CategoryLinear  Category = "linear"  // USDT/USDC perpetual
	CategoryInverse Category = "inverse" // Inverse perpetual
	CategoryOption  Category = "option"
)

// ProductSlugToCategory maps cpanel product slugs to Bybit categories
var ProductSlugToCategory = map[string]Category{
	"spot":    CategorySpot,
	"linear":  CategoryLinear,
	"inverse": CategoryInverse,
	"option":  CategoryOption,
}

// CategoryToWsURL maps categories to WebSocket URLs
var CategoryToWsURL = map[Category]string{
	CategorySpot:    WsURLSpot,
	CategoryLinear:  WsURLLinear,
	CategoryInverse: WsURLInverse,
	CategoryOption:  WsURLOption,
}

// DepthLevel represents the orderbook depth level for subscription
// Bybit supports different depths with different push frequencies
type DepthLevel int

const (
	// Linear & Inverse & Spot depths
	DepthLevel1    DepthLevel = 1    // 10ms push frequency
	DepthLevel50   DepthLevel = 50   // 20ms push frequency
	DepthLevel200  DepthLevel = 200  // 100ms (linear/inverse) or 200ms (spot)
	DepthLevel1000 DepthLevel = 1000 // 200ms push frequency

	// Option-specific depths
	DepthLevel25  DepthLevel = 25  // 20ms push frequency
	DepthLevel100 DepthLevel = 100 // 100ms push frequency
)

// DepthSubscriptionOptions holds options for depth subscription
type DepthSubscriptionOptions struct {
	Depth DepthLevel
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return "HTTP error: " + e.Body
}
