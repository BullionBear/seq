package binance

import (
	"strconv"
	"strings"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/valyala/fasthttp"
)

type BinanceHTTPClient struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus
	client   fasthttp.Client
	builder  strings.Builder
}

func NewBinanceHTTPClient(catalog *catalog.Catalog, eventBus *evbus.EventBus) BinanceHTTPClient {
	return BinanceHTTPClient{
		catalog: catalog,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
	}
}

// GetDepth fetches order book depth from Binance API
// Uses zero-allocation approach with strings.Builder for URL construction
func (c *BinanceHTTPClient) ReqDepth(symbolId int, limit int) error {
	// Build URL using strings.Builder (zero-allocation string concatenation)
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}
	c.builder.Reset()
	c.builder.Grow(len(BaseURL) + len(EndpointDepth) + len(symbol.Name) + 32) // Pre-allocate capacity
	c.builder.WriteString(BaseURL)
	c.builder.WriteString(EndpointDepth)
	c.builder.WriteByte('?')
	c.builder.WriteString("symbol=")
	c.builder.WriteString(symbol.Name)
	if limit > 0 {
		c.builder.WriteString("&limit=")
		c.builder.WriteString(strconv.Itoa(limit))
	}

	// Acquire request and response from fasthttp's internal pools (zero-allocation)
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Set request URI and method
	req.SetRequestURI(c.builder.String())
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	// Execute the request
	err = c.client.Do(req, resp)
	if err != nil {
		return err
	}

	// Check response status
	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		return &HTTPError{
			StatusCode: statusCode,
			Body:       string(resp.Body()),
		}
	}

	return nil
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return "HTTP error: " + strconv.Itoa(e.StatusCode) + " - " + e.Body
}
