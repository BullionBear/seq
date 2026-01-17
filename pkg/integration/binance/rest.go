package binance

import (
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

// Client represents a Binance REST API client with zero-allocation design
type Client struct {
	client    *fasthttp.Client
	baseURL   string
	parser    sync.Pool // Pool of fastjson.Parser instances
	depthResp sync.Pool // Pool of DepthResponse instances (ring buffer via sync.Pool)
}

// NewClient creates a new Binance REST API client
func NewClient() *Client {
	return NewClientWithURL(BaseURL)
}

// NewClientWithURL creates a new Binance REST API client with a custom base URL
func NewClientWithURL(baseURL string) *Client {
	return &Client{
		client: &fasthttp.Client{
			// Enable connection pooling for better performance and zero-allocation
			MaxConnsPerHost: 100,
		},
		baseURL: baseURL,
		parser: sync.Pool{
			New: func() interface{} {
				return &fastjson.Parser{}
			},
		},
		depthResp: sync.Pool{
			New: func() interface{} {
				return &DepthResponse{}
			},
		},
	}
}

// unsafeString converts []byte to string without allocation
// WARNING: The byte slice must not be modified after conversion
func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// GetDepth retrieves the order book depth for a symbol
func (c *Client) GetDepth(params DepthParams) (*DepthResponse, error) {
	// Acquire request and response from fasthttp's internal pools (zero-allocation)
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Build URL with query parameters using pooled byte buffer (zero-allocation)
	buf := bytebufferpool.Get()
	buf.WriteString(c.baseURL)
	buf.WriteString(EndpointDepth)
	req.SetRequestURIBytes(buf.B)
	bytebufferpool.Put(buf)
	uri := req.URI()

	// Build query string efficiently
	args := uri.QueryArgs()
	args.Set("symbol", params.Symbol)
	if params.Limit > 0 {
		args.Set("limit", strconv.Itoa(params.Limit))
	}

	// Set request method
	req.Header.SetMethod("GET")

	// Perform request (connection reuse handled by fasthttp.Client)
	if err := c.client.Do(req, resp); err != nil {
		return nil, err
	}

	// Check status code
	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		return nil, &APIError{
			StatusCode: statusCode,
			Body:       string(resp.Body()),
		}
	}

	// Parse JSON response using fastjson (zero-allocation parsing)
	body := resp.Body()
	parser := c.parser.Get().(*fastjson.Parser)
	defer c.parser.Put(parser)

	v, err := parser.ParseBytes(body)
	if err != nil {
		return nil, err
	}

	// Get DepthResponse from pool (zero-allocation via dispatcher pattern)
	depthResp := c.depthResp.Get().(*DepthResponse)
	depthResp.Reset() // Reset length counters

	depthResp.LastUpdateID = v.GetInt64("lastUpdateId")

	// Parse bids array into pre-allocated array
	bidsArray := v.GetArray("bids")
	if bidsArray != nil {
		maxBids := len(bidsArray)
		if maxBids > MaxDepthLevels {
			maxBids = MaxDepthLevels
		}
		depthResp.bidsLen = 0
		for i := 0; i < maxBids; i++ {
			bidItem := bidsArray[i]
			bidPair := bidItem.GetArray()
			if len(bidPair) >= 2 {
				priceBytes := bidPair[0].GetStringBytes()
				quantityBytes := bidPair[1].GetStringBytes()

				// Use unsafe.String to convert []byte to string without allocation
				priceStr := unsafeString(priceBytes)
				quantityStr := unsafeString(quantityBytes)

				// Parse strings to float64
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					c.depthResp.Put(depthResp) // Return to pool on error
					return nil, err
				}
				quantity, err := strconv.ParseFloat(quantityStr, 64)
				if err != nil {
					c.depthResp.Put(depthResp) // Return to pool on error
					return nil, err
				}

				// Store directly in pre-allocated array (zero-allocation)
				depthResp.bidsArray[depthResp.bidsLen] = PriceLevel{
					Price:    price,
					Quantity: quantity,
				}
				depthResp.bidsLen++
			}
		}
	}

	// Parse asks array into pre-allocated array
	asksArray := v.GetArray("asks")
	if asksArray != nil {
		maxAsks := len(asksArray)
		if maxAsks > MaxDepthLevels {
			maxAsks = MaxDepthLevels
		}
		depthResp.asksLen = 0
		for i := 0; i < maxAsks; i++ {
			askItem := asksArray[i]
			askPair := askItem.GetArray()
			if len(askPair) >= 2 {
				priceBytes := askPair[0].GetStringBytes()
				quantityBytes := askPair[1].GetStringBytes()

				// Use unsafe.String to convert []byte to string without allocation
				priceStr := unsafeString(priceBytes)
				quantityStr := unsafeString(quantityBytes)

				// Parse strings to float64
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					c.depthResp.Put(depthResp) // Return to pool on error
					return nil, err
				}
				quantity, err := strconv.ParseFloat(quantityStr, 64)
				if err != nil {
					c.depthResp.Put(depthResp) // Return to pool on error
					return nil, err
				}

				// Store directly in pre-allocated array (zero-allocation)
				depthResp.asksArray[depthResp.asksLen] = PriceLevel{
					Price:    price,
					Quantity: quantity,
				}
				depthResp.asksLen++
			}
		}
	}

	// Note: Caller should call ReleaseDepthResponse when done with the response
	return depthResp, nil
}

// APIError represents an API error response
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return strings.TrimSpace(e.Body)
}

// ReleaseDepthResponse returns a DepthResponse to the pool for reuse
// Call this when you're done with a DepthResponse to enable zero-allocation reuse
func (c *Client) ReleaseDepthResponse(resp *DepthResponse) {
	if resp != nil {
		c.depthResp.Put(resp)
	}
}
