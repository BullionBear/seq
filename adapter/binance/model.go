package binance

import "strconv"

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return "HTTP error: " + strconv.Itoa(e.StatusCode) + " - " + e.Body
}

type PushRate int

const (
	PushRate100ms PushRate = iota
	PushRate1s
)

// DepthSubscriptionOptions holds options for depth subscription.
// Levels 0 means diff-depth stream; 5, 10, or 20 means partial-book stream.
type DepthSubscriptionOptions struct {
	PushRate PushRate
	Levels   int // 0 = diff (@depth@…), 5|10|20 = partial (@depthN@…)
}

// IsPartialBook reports whether this subscription uses a partial-book stream.
func (o *DepthSubscriptionOptions) IsPartialBook() bool {
	return o != nil && (o.Levels == 5 || o.Levels == 10 || o.Levels == 20)
}

// StreamSuffix returns the Binance depth stream suffix for the given push rate
// and book levels. Diff depth: "depth@100ms". Partial: "depth20@100ms".
func (p PushRate) StreamSuffix(levels int) string {
	rate := "1000ms"
	if p == PushRate100ms {
		rate = "100ms"
	}
	switch levels {
	case 5, 10, 20:
		return "depth" + strconv.Itoa(levels) + "@" + rate
	default:
		return "depth@" + rate
	}
}

// TradeSubscriptionOptions holds options for trade subscription
type TradeSubscriptionOptions struct {
	UseAggTrade bool // true for aggTrade stream, false for trade stream
}
