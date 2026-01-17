package lynxlinkage

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

type LynxLinkageClient struct {
	baseURL string
	client  *fasthttp.Client
}

func NewLynxLinkageClient(baseURL string) *LynxLinkageClient {
	return &LynxLinkageClient{
		baseURL: baseURL,
		client: &fasthttp.Client{
			MaxConnsPerHost: 100,
		},
	}
}

// GetSymbol fetches symbols from GET /api/v1/symbol endpoint
// It supports optional query parameters via SymbolParams
func (c *LynxLinkageClient) GetSymbol(ctx context.Context, params SymbolParams) (*SymbolResponse, error) {
	// Build the URL with query parameters
	reqURL := fmt.Sprintf("%s/api/v1/symbol", c.baseURL)
	u, err := url.Parse(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if params.ExchangeID != nil {
		q.Set("exchange_id", strconv.Itoa(*params.ExchangeID))
	}
	if params.ExchangeSlug != nil {
		q.Set("exchange_slug", *params.ExchangeSlug)
	}
	if params.ProductID != nil {
		q.Set("product_id", strconv.Itoa(*params.ProductID))
	}
	if params.ProductSlug != nil {
		q.Set("product_slug", *params.ProductSlug)
	}
	if params.BaseTokenID != nil {
		q.Set("base_token_id", strconv.Itoa(*params.BaseTokenID))
	}
	if params.QuoteTokenID != nil {
		q.Set("quote_token_id", strconv.Itoa(*params.QuoteTokenID))
	}

	u.RawQuery = q.Encode()
	reqURL = u.String()

	// Acquire request and response objects from pool
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Set request URI and method
	req.SetRequestURI(reqURL)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	// Set timeout from context if available
	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, fmt.Errorf("context deadline exceeded")
		}
	}

	// Execute the request
	err = c.client.DoTimeout(req, resp, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// Check response status
	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", statusCode, resp.Body())
	}

	// Deserialize response using sonic
	var result SymbolResponse
	err = sonic.Unmarshal(resp.Body(), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}
