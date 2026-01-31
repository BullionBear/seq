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

// StreamSuffix returns the Binance stream suffix for the push rate
func (p PushRate) StreamSuffix() string {
	switch p {
	case PushRate100ms:
		return "depth@100ms"
	case PushRate1s:
		return "depth@1000ms"
	default:
		return "depth@1000ms" // default to 1s
	}
}

type DepthSubscriptionOptions struct {
	PushRate PushRate
}
