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

// binanceDepthResponse represents the JSON response from Binance depth API
type binanceDepthResponse struct {
	LastUpdateID int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}
