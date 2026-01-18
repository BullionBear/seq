package binance

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unsafe"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/BullionBear/seq/pkg/model"
	"github.com/valyala/fasthttp"
)

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type BinanceHTTPClient struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus
	client   fasthttp.Client
	builder  strings.Builder
}

func NewBinanceHTTPClient(catalog *catalog.Catalog, eventBus *evbus.EventBus) BinanceHTTPClient {
	return BinanceHTTPClient{
		catalog:  catalog,
		eventBus: eventBus,
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

	// Deserialize depth response
	var depth model.DepthSnapshot
	err = c.unmarshalDepthSnapshot(resp.Body(), &depth)
	if err != nil {
		return err
	}

	// Publish to event bus
	c.eventBus.PublishDepthSnapshot(depth)
	return nil
}

func (c *BinanceHTTPClient) unmarshalDepthSnapshot(data []byte, depth *model.DepthSnapshot) error {
	// Parse lastUpdateId
	const lastUpdateIdKey = "\"lastUpdateId\""
	idx := bytes.Index(data, []byte(lastUpdateIdKey))
	if idx == -1 {
		return errors.New("missing lastUpdateId")
	}

	// Move past key
	idx += len(lastUpdateIdKey)
	// Find colon
	for idx < len(data) && data[idx] != ':' {
		idx++
	}
	idx++ // skip colon

	// Skip whitespace
	for idx < len(data) && (data[idx] == ' ' || data[idx] == '\n' || data[idx] == '\t') {
		idx++
	}

	// Parse integer for lastUpdateId
	start := idx
	for idx < len(data) && data[idx] >= '0' && data[idx] <= '9' {
		idx++
	}
	// Note: We're not storing lastUpdateId in model.DepthSnapshot per the definition provided earlier,
	// but the user requirement implies we are "implementing UnmarshalDepthSnapshot data".
	// The provided model.DepthSnapshot has DepthID int, Timestamp uint64 etc.
	// Looking at evbus.go: type DepthSnapshot struct { ... DepthID int ... }
	// So we assume DepthID maps to lastUpdateId.

	if start == idx {
		return errors.New("invalid lastUpdateId value")
	}

	val, err := strconv.Atoi(unsafeString(data[start:idx]))
	if err != nil {
		return err
	}
	depth.DepthID = val

	// Helper to parse double array [[string, string], ...]
	// We do 2 passes: 1 to count, 2 to fill
	parseOrderList := func(key string) ([]model.PriceLevel, error) {
		keyIdx := bytes.Index(data, []byte(key))
		if keyIdx == -1 {
			return nil, nil // or error? standard json unmarshal puts empty if missing
		}

		// Find start of array [
		curr := keyIdx + len(key)
		for curr < len(data) && data[curr] != '[' {
			curr++
		}
		if curr >= len(data) {
			return nil, errors.New("array start not found for " + key)
		}
		arrayStart := curr

		// Pass 1: Count
		count := 0
		curr++ // skip [

		// This is a naive count. We assume structure [[...],[...]]
		// We can just count occurrence of "],[" maybe? Or just count '[' after the first one?
		// Binance format: "bids": [ [ "4.00", "12.00"], [ "3.00", "10.00" ] ]

		scanIdx := curr
		depth := 0
		for scanIdx < len(data) {
			b := data[scanIdx]
			if b == '[' {
				depth++
				if depth == 1 {
					count++
				}
			} else if b == ']' {
				depth--
				if depth < 0 {
					break // End of outer array
				}
			}
			scanIdx++
		}

		if count == 0 {
			return nil, nil
		}

		// Alloc
		levels := c.eventBus.AllocPriceLevels(count)

		// Pass 2: Parse
		curr = arrayStart + 1
		itemIdx := 0

		// We expect: [ "price", "qty" ] ...
		for itemIdx < count {
			// Find start of subarray
			for curr < len(data) && data[curr] != '[' {
				curr++
			}
			curr++ // skip [

			// Parse Price
			// Find quote
			for curr < len(data) && data[curr] != '"' {
				curr++
			}
			curr++ // skip "
			pStart := curr
			for curr < len(data) && data[curr] != '"' {
				curr++
			}
			pStr := unsafeString(data[pStart:curr])
			price, err := strconv.ParseFloat(pStr, 64)
			if err != nil {
				return nil, err
			}
			curr++ // skip closing "

			// Find next value (Quantity)
			for curr < len(data) && data[curr] != '"' {
				curr++
			}
			curr++ // skip "
			qStart := curr
			for curr < len(data) && data[curr] != '"' {
				curr++
			}
			qStr := unsafeString(data[qStart:curr])
			qty, err := strconv.ParseFloat(qStr, 64)
			if err != nil {
				return nil, err
			}
			curr++ // skip closing "

			levels[itemIdx] = model.PriceLevel{
				Price:    price,
				Quantity: qty,
			}
			itemIdx++

			// Move past current subarray closure ]
			for curr < len(data) && data[curr] != ']' {
				curr++
			}
			curr++ // skip ]
		}

		return levels, nil
	}

	depth.Bids, err = parseOrderList("\"bids\"")
	if err != nil {
		return err
	}

	depth.Asks, err = parseOrderList("\"asks\"")
	if err != nil {
		return err
	}

	return nil
}

func (c *BinanceHTTPClient) ReqCreateOrder(acctID int, symbolId int, orderType model.OrderType, side model.Side, timeInForce model.TimeInForce, quantity float64, price float64) error {
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}

	acct, err := c.catalog.GetAccount(acctID)
	if err != nil {
		return err
	}
}
