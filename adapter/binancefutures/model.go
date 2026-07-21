package binancefutures

import "strconv"

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return "HTTP error: " + strconv.Itoa(e.StatusCode) + " - " + e.Body
}

// PushRate is the Binance USD-M futures depth push interval.
type PushRate int

const (
	PushRate100ms PushRate = iota
	PushRate250ms
	PushRate500ms
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

// StreamSuffix returns the Binance futures depth stream suffix for the given
// push rate and book levels. Diff: "depth@100ms". Partial: "depth20@100ms".
func (p PushRate) StreamSuffix(levels int) string {
	rate := "100ms"
	switch p {
	case PushRate250ms:
		rate = "250ms"
	case PushRate500ms:
		rate = "500ms"
	}
	switch levels {
	case 5, 10, 20:
		return "depth" + strconv.Itoa(levels) + "@" + rate
	default:
		return "depth@" + rate
	}
}

// pushRateFromMs maps YAML push_rate milliseconds onto futures-supported rates.
// Futures offers 100ms / 250ms / 500ms; values >=1000 map to 500ms.
func pushRateFromMs(pushRateMs int) PushRate {
	switch {
	case pushRateMs >= 1000:
		return PushRate500ms
	case pushRateMs >= 250:
		return PushRate250ms
	default:
		return PushRate100ms
	}
}

