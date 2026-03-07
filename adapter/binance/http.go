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
	"github.com/BullionBear/seq/core/msgbus"

	"github.com/valyala/fasthttp"
)

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type BinanceHTTPClient struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	client  fasthttp.Client
	buffer  bytes.Buffer
	baseURL string
}

func NewBinanceHTTPClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) BinanceHTTPClient {
	return BinanceHTTPClient{
		catalog: catalog,
		msgBus:  msgBus,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
		baseURL: BaseURL,
	}
}

// GetDepth fetches order book depth from Binance API
// Uses zero-allocation approach: parses JSON directly into arena buffer
func (c *BinanceHTTPClient) ReqDepthSnapshot(symbolId int, limit int) error {
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

	jsonData := resp.Body()

	// Phase 1: Parse header and count price levels (no allocation)
	depthID, askCount, bidCount, err := c.countDepthLevels(jsonData)
	if err != nil {
		return err
	}

	// Phase 2: Allocate arena buffer for DepthSnapshot
	// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
	size := uint64(event.DepthSnapshotHeaderSize) +
		uint64(askCount)*uint64(event.PriceLevelSize) +
		uint64(bidCount)*uint64(event.PriceLevelSize)

	offset, buf := c.msgBus.Allocate(size)

	// Phase 3: Parse directly into arena buffer (zero-allocation)
	// Get pointers to the PriceLevel arrays in the buffer
	asksStart := event.DepthSnapshotHeaderSize
	bidsStart := asksStart + askCount*event.PriceLevelSize

	var asks []common.PriceLevel
	var bids []common.PriceLevel
	if askCount > 0 {
		asks = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[asksStart])), askCount)
	}
	if bidCount > 0 {
		bids = unsafe.Slice((*common.PriceLevel)(unsafe.Pointer(&buf[bidsStart])), bidCount)
	}

	// Parse price levels directly into arena with precision for PriceTick/QuantityTick
	err = c.parsePriceLevelsInto(jsonData, "\"asks\"", asks, symbol.PricePrecision, symbol.SizePrecision)
	if err != nil {
		return err
	}
	err = c.parsePriceLevelsInto(jsonData, "\"bids\"", bids, symbol.PricePrecision, symbol.SizePrecision)
	if err != nil {
		return err
	}

	// Encode snapshot into buffer (header + self-copy of price levels already in buffer)
	timestamp := uint64(time.Now().UnixNano())
	snapshot := event.DepthSnapshot{
		SymbolID:  symbolId,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
	snapshot.Encode(buf)

	// Publish to event bus
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventDepthSnapshot,
		Index:  offset,
		Length: size,
	})
	return nil
}

// countDepthLevels parses JSON to extract depthID and count asks/bids (first pass, no allocation)
func (c *BinanceHTTPClient) countDepthLevels(data []byte) (depthID, askCount, bidCount int, err error) {
	// Parse lastUpdateId
	const lastUpdateIdKey = "\"lastUpdateId\""
	idx := bytes.Index(data, []byte(lastUpdateIdKey))
	if idx == -1 {
		return 0, 0, 0, errors.New("missing lastUpdateId")
	}

	// Move past key and find colon
	idx += len(lastUpdateIdKey)
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
	if start == idx {
		return 0, 0, 0, errors.New("invalid lastUpdateId value")
	}

	depthID, err = strconv.Atoi(unsafeString(data[start:idx]))
	if err != nil {
		return 0, 0, 0, err
	}

	// Count asks
	askCount = c.countArrayElements(data, "\"asks\"")

	// Count bids
	bidCount = c.countArrayElements(data, "\"bids\"")

	return depthID, askCount, bidCount, nil
}

// countArrayElements counts the number of sub-arrays in a JSON array
func (c *BinanceHTTPClient) countArrayElements(data []byte, key string) int {
	keyIdx := bytes.Index(data, []byte(key))
	if keyIdx == -1 {
		return 0
	}

	// Find start of array [
	curr := keyIdx + len(key)
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return 0
	}
	curr++ // skip [

	// Count sub-arrays by tracking depth
	count := 0
	depth := 0
	for curr < len(data) {
		b := data[curr]
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
		curr++
	}

	return count
}

// parsePriceLevelsInto parses price levels directly into a pre-allocated slice (zero-allocation).
// Uses pricePrecision and sizePrecision to compute PriceTick and QuantityTick.
func (c *BinanceHTTPClient) parsePriceLevelsInto(data []byte, key string, levels []common.PriceLevel, pricePrecision, sizePrecision int) error {
	if len(levels) == 0 {
		return nil
	}

	keyIdx := bytes.Index(data, []byte(key))
	if keyIdx == -1 {
		return nil
	}

	// Find start of array [
	curr := keyIdx + len(key)
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return errors.New("array start not found for " + key)
	}
	curr++ // skip [

	// Parse each price level
	for itemIdx := 0; itemIdx < len(levels); itemIdx++ {
		// Find start of subarray
		for curr < len(data) && data[curr] != '[' {
			curr++
		}
		curr++ // skip [

		// Parse Price - find quote
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		curr++ // skip "
		pStart := curr
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		price, err := strconv.ParseFloat(unsafeString(data[pStart:curr]), 64)
		if err != nil {
			return err
		}
		curr++ // skip closing "

		// Parse Quantity - find quote
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		curr++ // skip "
		qStart := curr
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		qty, err := strconv.ParseFloat(unsafeString(data[qStart:curr]), 64)
		if err != nil {
			return err
		}
		curr++ // skip closing "

		// Write directly into the pre-allocated slice (which is in arena)
		levels[itemIdx].Price = price
		levels[itemIdx].Quantity = qty
		levels[itemIdx].PriceTick = common.PriceToTick(price, pricePrecision)
		levels[itemIdx].QuantityTick = common.QuantityToTick(qty, sizePrecision)

		// Move past current subarray closure ]
		for curr < len(data) && data[curr] != ']' {
			curr++
		}
		curr++ // skip ]
	}

	return nil
}

// unmarshalRespDepthSnapshot parses JSON into RespDepthSnapshot (allocates slices - use for testing)
func (c *BinanceHTTPClient) unmarshalRespDepthSnapshot(data []byte, respDepthSnapshot *event.RespDepthSnapshot) error {
	// Parse header and count levels
	depthID, askCount, bidCount, err := c.countDepthLevels(data)
	if err != nil {
		return err
	}
	respDepthSnapshot.DepthID = depthID
	respDepthSnapshot.AskLength = askCount
	respDepthSnapshot.BidLength = bidCount

	// Allocate slices (non-zero-allocation path, for testing)
	// Use default precisions 2,5 for testing when symbol not available
	if askCount > 0 {
		respDepthSnapshot.Asks = make([]common.PriceLevel, askCount)
		if err := c.parsePriceLevelsInto(data, "\"asks\"", respDepthSnapshot.Asks, 2, 5); err != nil {
			return err
		}
	}
	if bidCount > 0 {
		respDepthSnapshot.Bids = make([]common.PriceLevel, bidCount)
		if err := c.parsePriceLevelsInto(data, "\"bids\"", respDepthSnapshot.Bids, 2, 5); err != nil {
			return err
		}
	}

	return nil
}

func (c *BinanceHTTPClient) ReqCreateOrder(acctID int, apiKeyName string, symbolId int, orderType common.OrderType, side common.Side, timeInForce common.TimeInForce, quantity float64, price float64) error {
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}

	acct, err := c.catalog.GetAccount(acctID)
	if err != nil {
		return err
	}

	apiKey, err := acct.GetAPI(apiKeyName)
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
	signature := c.hmacSha256(c.buffer.Bytes()[queryStart:], apiKey.Secret)
	c.buffer.WriteString("&signature=")
	c.buffer.WriteString(signature)

	// Prepare request
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Method POST
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.Set("X-MBX-APIKEY", apiKey.Key)
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
