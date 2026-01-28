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

type DepthSubscriptionOptions struct {
	SymbolID int
	Limit    int
}
