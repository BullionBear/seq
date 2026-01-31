package binance

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"

	"github.com/valyala/fasthttp"
)

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type BinanceHTTPClient struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus
	client   fasthttp.Client
	buffer   bytes.Buffer
	baseURL  string
}

func NewBinanceHTTPClient(catalog *catalog.Catalog, eventBus *evbus.EventBus) BinanceHTTPClient {
	return BinanceHTTPClient{
		catalog:  catalog,
		eventBus: eventBus,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
		baseURL: BaseURL,
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
	c.buffer.Reset()
	c.buffer.Grow(len(c.baseURL) + len(EndpointDepth) + len(symbol.Name) + 32)
	c.buffer.WriteString(c.baseURL)
	c.buffer.WriteString(EndpointDepth)
	c.buffer.WriteByte('?')
	c.buffer.WriteString("symbol=")
	c.buffer.WriteString(symbol.Name)
	if limit > 0 {
		c.buffer.WriteString("&limit=")
		c.buffer.WriteString(strconv.Itoa(limit))
	}

	// Acquire request and response from fasthttp's internal pools (zero-allocation)
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Set request URI and method
	// Set request URI and method
	req.SetRequestURIBytes(c.buffer.Bytes())
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
	var depth event.DepthSnapshot
	err = c.unmarshalDepthSnapshot(resp.Body(), &depth)
	if err != nil {
		return err
	}

	// Set symbolID and timestamp
	depth.SymbolID = symbolId
	depth.Timestamp = uint64(time.Now().UnixNano())

	// Publish to event bus
	c.eventBus.PublishDepthSnapshot(depth)
	return nil
}

func (c *BinanceHTTPClient) unmarshalDepthSnapshot(data []byte, depth *event.DepthSnapshot) error {
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
	parseOrderList := func(key string) ([]event.PriceLevel, error) {
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

		// Allocate slice for price levels
		levels := make([]event.PriceLevel, count)

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

			levels[itemIdx] = event.PriceLevel{
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

func (c *BinanceHTTPClient) ReqCreateOrder(acctID int, symbolId int, orderType common.OrderType, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) error {
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}

	acct, err := c.catalog.GetAccount(acctID)
	if err != nil {
		return err
	}

	// Build query string
	c.buffer.Reset()
	c.buffer.Grow(256) // Heuristic alloc

	c.buffer.WriteString(c.baseURL)
	c.buffer.WriteString(EndpointCreateOrder)
	c.buffer.WriteByte('?')

	queryStart := c.buffer.Len()

	// symbol
	c.buffer.WriteString("symbol=")
	c.buffer.WriteString(symbol.Name)

	// side
	c.buffer.WriteString("&side=")
	switch side {
	case common.SideBuy:
		c.buffer.WriteString("BUY")
	case common.SideSell:
		c.buffer.WriteString("SELL")
	}

	// type
	c.buffer.WriteString("&type=")
	isLimitMaker := false
	switch orderType {
	case common.OrderTypeLimit:
		if timeInForce == common.TimeInForcePO {
			c.buffer.WriteString("LIMIT_MAKER")
			isLimitMaker = true
		} else {
			c.buffer.WriteString("LIMIT")
		}
	case common.OrderTypeMarket:
		c.buffer.WriteString("MARKET")
	}

	// timeInForce
	if orderType == common.OrderTypeLimit && !isLimitMaker {
		c.buffer.WriteString("&timeInForce=")
		switch timeInForce {
		case common.TimeInForceGTC:
			c.buffer.WriteString("GTC")
		case common.TimeInForceIOC:
			c.buffer.WriteString("IOC")
		case common.TimeInForceFOK:
			c.buffer.WriteString("FOK")
		}
	}

	// quantity
	c.buffer.WriteString("&quantity=")
	c.buffer.WriteString(strconv.FormatFloat(quantity, 'f', -1, 64))

	// price
	if orderType == common.OrderTypeLimit {
		c.buffer.WriteString("&price=")
		c.buffer.WriteString(strconv.FormatFloat(price, 'f', -1, 64))
	}

	// timestamp
	c.buffer.WriteString("&timestamp=")
	c.buffer.WriteString(strconv.FormatInt(time.Now().UnixMilli(), 10))

	// recvWindow
	c.buffer.WriteString("&recvWindow=5000")

	// newOrderRespType
	c.buffer.WriteString("&newOrderRespType=ACK")

	// Sign
	signature := c.hmacSha256(c.buffer.Bytes()[queryStart:], acct.APISecret)
	c.buffer.WriteString("&signature=")
	c.buffer.WriteString(signature)

	// Prepare request
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Method POST
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.Set("X-MBX-APIKEY", acct.APIKey)
	req.Header.SetContentType("application/x-www-form-urlencoded")

	// URL
	req.SetRequestURIBytes(c.buffer.Bytes())

	err = c.client.Do(req, resp)
	if err != nil {
		return err
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return &HTTPError{
			StatusCode: resp.StatusCode(),
			Body:       string(resp.Body()),
		}
	}

	return nil
}

func (c *BinanceHTTPClient) hmacSha256(data []byte, secret string) string {
	h := hmac.New(sha256.New, unsafeStringBytes(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func unsafeStringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
