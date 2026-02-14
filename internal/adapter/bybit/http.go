package bybit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/msgbus"

	"github.com/valyala/fasthttp"
)

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type BybitHTTPClient struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	client   fasthttp.Client
	buffer   bytes.Buffer
	baseURL  string
}

func NewBybitHTTPClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) BybitHTTPClient {
	return BybitHTTPClient{
		catalog: catalog,
		msgBus:  msgBus,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
		baseURL: BaseURL,
	}
}

// ReqDepthSnapshot fetches order book depth from Bybit API
// Bybit V5 API unifies all products - just change the "category" parameter
// Uses zero-allocation approach: parses JSON directly into arena buffer
func (c *BybitHTTPClient) ReqDepthSnapshot(symbolId int, limit int) error {
	// Get symbol info from catalog
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}

	// Map product slug to Bybit category
	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		return errors.New("unsupported product type for Bybit: " + symbol.Product.Slug)
	}

	// Build URL using bytes.Buffer (zero-allocation string concatenation)
	// Bybit endpoint: /v5/market/orderbook?category=spot&symbol=BTCUSDT&limit=50
	c.buffer.Reset()
	c.buffer.Grow(len(c.baseURL) + len(EndpointOrderbook) + len(symbol.Name) + 64)
	c.buffer.WriteString(c.baseURL)
	c.buffer.WriteString(EndpointOrderbook)
	c.buffer.WriteByte('?')
	c.buffer.WriteString("category=")
	c.buffer.WriteString(string(category))
	c.buffer.WriteString("&symbol=")
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

	// Check Bybit API response code (retCode should be 0 for success)
	if err := c.checkRetCode(jsonData); err != nil {
		return err
	}

	// Phase 1: Parse header and count price levels (no allocation)
	depthID, timestamp, askCount, bidCount, err := c.countDepthLevels(jsonData)
	if err != nil {
		return err
	}

	// Phase 2: Allocate arena buffer for DepthSnapshot
	// Layout: [SymbolID(8)][DepthID(8)][Timestamp(8)][AsksLen(4)][BidsLen(4)][Asks...][Bids...]
	size := uint64(msgbus.SizeOfDepthSnapshotHeader) +
		uint64(askCount)*uint64(msgbus.SizeOfPriceLevel) +
		uint64(bidCount)*uint64(msgbus.SizeOfPriceLevel)

	offset, buf := c.msgBus.Allocate(size)

	// Phase 3: Parse directly into arena buffer (zero-allocation)
	// Get pointers to the PriceLevel arrays in the buffer
	asksStart := msgbus.SizeOfDepthSnapshotHeader
	bidsStart := asksStart + askCount*msgbus.SizeOfPriceLevel

	var asks []event.PriceLevel
	var bids []event.PriceLevel
	if askCount > 0 {
		asks = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[asksStart])), askCount)
	}
	if bidCount > 0 {
		bids = unsafe.Slice((*event.PriceLevel)(unsafe.Pointer(&buf[bidsStart])), bidCount)
	}

	// Parse price levels directly into arena
	// Bybit uses "a" for asks and "b" for bids
	err = c.parsePriceLevelsInto(jsonData, "\"a\"", asks)
	if err != nil {
		return err
	}
	err = c.parsePriceLevelsInto(jsonData, "\"b\"", bids)
	if err != nil {
		return err
	}

	// Write header into buffer
	// Convert Bybit timestamp (milliseconds) to nanoseconds
	timestampNano := timestamp * 1000000
	msgbus.WriteDepthSnapshotHeader(buf, symbolId, depthID, timestampNano, uint32(askCount), uint32(bidCount))

	// Publish to event bus
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventDepthSnapshot,
		Index:  offset,
		Length: size,
	})
	return nil
}

// checkRetCode verifies Bybit API response has retCode=0
func (c *BybitHTTPClient) checkRetCode(data []byte) error {
	const retCodeKey = "\"retCode\""
	idx := bytes.Index(data, []byte(retCodeKey))
	if idx == -1 {
		return errors.New("missing retCode in response")
	}

	// Move past key and find colon
	idx += len(retCodeKey)
	for idx < len(data) && data[idx] != ':' {
		idx++
	}
	idx++ // skip colon

	// Skip whitespace
	for idx < len(data) && (data[idx] == ' ' || data[idx] == '\n' || data[idx] == '\t') {
		idx++
	}

	// Parse integer for retCode
	start := idx
	for idx < len(data) && data[idx] >= '0' && data[idx] <= '9' {
		idx++
	}
	if start == idx {
		return errors.New("invalid retCode value")
	}

	retCode, err := strconv.Atoi(unsafeString(data[start:idx]))
	if err != nil {
		return err
	}

	if retCode != 0 {
		// Try to extract retMsg for better error message
		const retMsgKey = "\"retMsg\""
		msgIdx := bytes.Index(data, []byte(retMsgKey))
		if msgIdx != -1 {
			msgIdx += len(retMsgKey)
			for msgIdx < len(data) && data[msgIdx] != '"' {
				msgIdx++
			}
			msgIdx++ // skip opening quote
			msgEnd := msgIdx
			for msgEnd < len(data) && data[msgEnd] != '"' {
				msgEnd++
			}
			return errors.New("Bybit API error: " + unsafeString(data[msgIdx:msgEnd]))
		}
		return errors.New("Bybit API error: retCode=" + strconv.Itoa(retCode))
	}

	return nil
}

// countDepthLevels parses JSON to extract depthID (u), timestamp (ts), and count asks/bids
// Bybit response format: {"result": {"s": "BTCUSDT", "a": [...], "b": [...], "ts": 123, "u": 456}}
func (c *BybitHTTPClient) countDepthLevels(data []byte) (depthID int, timestamp uint64, askCount, bidCount int, err error) {
	// Find "result" object first
	const resultKey = "\"result\""
	resultIdx := bytes.Index(data, []byte(resultKey))
	if resultIdx == -1 {
		return 0, 0, 0, 0, errors.New("missing result in response")
	}

	// Work within the result object
	resultData := data[resultIdx:]

	// Parse "u" (update ID / depth ID)
	const updateIdKey = "\"u\""
	idx := bytes.Index(resultData, []byte(updateIdKey))
	if idx == -1 {
		return 0, 0, 0, 0, errors.New("missing u (update ID)")
	}

	// Move past key and find colon
	idx += len(updateIdKey)
	for idx < len(resultData) && resultData[idx] != ':' {
		idx++
	}
	idx++ // skip colon

	// Skip whitespace
	for idx < len(resultData) && (resultData[idx] == ' ' || resultData[idx] == '\n' || resultData[idx] == '\t') {
		idx++
	}

	// Parse integer for u
	start := idx
	for idx < len(resultData) && resultData[idx] >= '0' && resultData[idx] <= '9' {
		idx++
	}
	if start == idx {
		return 0, 0, 0, 0, errors.New("invalid u value")
	}

	depthID, err = strconv.Atoi(unsafeString(resultData[start:idx]))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Parse "ts" (timestamp in milliseconds)
	const tsKey = "\"ts\""
	tsIdx := bytes.Index(resultData, []byte(tsKey))
	if tsIdx == -1 {
		// Fallback to current time if ts not found
		timestamp = uint64(time.Now().UnixMilli())
	} else {
		tsIdx += len(tsKey)
		for tsIdx < len(resultData) && resultData[tsIdx] != ':' {
			tsIdx++
		}
		tsIdx++ // skip colon

		// Skip whitespace
		for tsIdx < len(resultData) && (resultData[tsIdx] == ' ' || resultData[tsIdx] == '\n' || resultData[tsIdx] == '\t') {
			tsIdx++
		}

		// Parse integer for ts
		tsStart := tsIdx
		for tsIdx < len(resultData) && resultData[tsIdx] >= '0' && resultData[tsIdx] <= '9' {
			tsIdx++
		}
		if tsStart == tsIdx {
			timestamp = uint64(time.Now().UnixMilli())
		} else {
			ts, err := strconv.ParseUint(unsafeString(resultData[tsStart:tsIdx]), 10, 64)
			if err != nil {
				timestamp = uint64(time.Now().UnixMilli())
			} else {
				timestamp = ts
			}
		}
	}

	// Count asks (Bybit uses "a" key)
	askCount = c.countArrayElements(resultData, "\"a\"")

	// Count bids (Bybit uses "b" key)
	bidCount = c.countArrayElements(resultData, "\"b\"")

	return depthID, timestamp, askCount, bidCount, nil
}

// countArrayElements counts the number of sub-arrays in a JSON array
func (c *BybitHTTPClient) countArrayElements(data []byte, key string) int {
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

// parsePriceLevelsInto parses price levels directly into a pre-allocated slice (zero-allocation)
// Bybit format: ["price", "size"] e.g., ["65557.7", "16.606555"]
func (c *BybitHTTPClient) parsePriceLevelsInto(data []byte, key string, levels []event.PriceLevel) error {
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

		// Move past current subarray closure ]
		for curr < len(data) && data[curr] != ']' {
			curr++
		}
		curr++ // skip ]
	}

	return nil
}

// unmarshalReqDepthSnapshot parses JSON into ReqDepthSnapshot (allocates slices - use for testing)
func (c *BybitHTTPClient) unmarshalReqDepthSnapshot(data []byte, reqDepthSnapshot *event.ReqDepthSnapshot) error {
	// Check Bybit API response code first
	if err := c.checkRetCode(data); err != nil {
		return err
	}

	// Parse header and count levels
	depthID, timestamp, askCount, bidCount, err := c.countDepthLevels(data)
	if err != nil {
		return err
	}
	reqDepthSnapshot.DepthID = depthID
	reqDepthSnapshot.Timestamp = timestamp * 1000000 // convert ms to ns
	reqDepthSnapshot.AskLength = askCount
	reqDepthSnapshot.BidLength = bidCount

	// Find "result" object for parsing price levels
	const resultKey = "\"result\""
	resultIdx := bytes.Index(data, []byte(resultKey))
	if resultIdx == -1 {
		return errors.New("missing result in response")
	}
	resultData := data[resultIdx:]

	// Allocate slices (non-zero-allocation path, for testing)
	if askCount > 0 {
		reqDepthSnapshot.Asks = make([]event.PriceLevel, askCount)
		if err := c.parsePriceLevelsInto(resultData, "\"a\"", reqDepthSnapshot.Asks); err != nil {
			return err
		}
	}
	if bidCount > 0 {
		reqDepthSnapshot.Bids = make([]event.PriceLevel, bidCount)
		if err := c.parsePriceLevelsInto(resultData, "\"b\"", reqDepthSnapshot.Bids); err != nil {
			return err
		}
	}

	return nil
}

// ============================================================================
// Authenticated Endpoints
// ============================================================================

// ReqBalanceSnapshot fetches account wallet balance from Bybit API
// Endpoint: GET /v5/account/wallet-balance
// This is an authenticated endpoint requiring HMAC signature
//
// Bybit response format:
//
//	{
//	  "retCode": 0,
//	  "retMsg": "OK",
//	  "result": {
//	    "list": [{
//	      "accountType": "UNIFIED",
//	      "coin": [{
//	        "coin": "BTC",
//	        "walletBalance": "0.00000001",
//	        "availableToWithdraw": "0.00000001",
//	        "locked": "0"
//	      }]
//	    }]
//	  }
//	}
func (c *BybitHTTPClient) ReqBalanceSnapshot(accountID int, accountType string) error {
	// Get account credentials from catalog
	account, err := c.catalog.GetAccount(accountID)
	if err != nil {
		return err
	}

	// Default to UNIFIED account type if not specified
	if accountType == "" {
		accountType = "UNIFIED"
	}

	// Build query string
	queryString := "accountType=" + accountType

	// Generate timestamp
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// Build signature payload: timestamp + api_key + recv_window + query_string
	c.buffer.Reset()
	c.buffer.WriteString(timestamp)
	c.buffer.WriteString(account.APIKey)
	c.buffer.WriteString(HTTPRecvWindow)
	c.buffer.WriteString(queryString)

	// Sign with HMAC-SHA256
	signature := c.signHMAC(c.buffer.Bytes(), account.APISecret)

	// Build full URL
	c.buffer.Reset()
	c.buffer.WriteString(c.baseURL)
	c.buffer.WriteString(EndpointWalletBalance)
	c.buffer.WriteByte('?')
	c.buffer.WriteString(queryString)

	// Acquire request and response from fasthttp's internal pools
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Set request URI and method
	req.SetRequestURIBytes(c.buffer.Bytes())
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	// Set authentication headers
	req.Header.Set("X-BAPI-API-KEY", account.APIKey)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-SIGN", signature)
	req.Header.Set("X-BAPI-RECV-WINDOW", HTTPRecvWindow)

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

	// Check Bybit API response code
	if err := c.checkRetCode(jsonData); err != nil {
		return err
	}

	// Parse and publish balance snapshot
	return c.parseAndPublishBalanceSnapshot(jsonData, accountID)
}

// signHMAC signs the payload with HMAC-SHA256
func (c *BybitHTTPClient) signHMAC(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// parseAndPublishBalanceSnapshot parses wallet balance response and publishes to event bus
func (c *BybitHTTPClient) parseAndPublishBalanceSnapshot(data []byte, accountID int) error {
	// Find "list" array in result
	const listKey = "\"list\""
	listIdx := bytes.Index(data, []byte(listKey))
	if listIdx == -1 {
		return errors.New("missing list in response")
	}

	// Find "coin" array within the first account in list
	const coinKey = "\"coin\""
	coinIdx := bytes.Index(data[listIdx:], []byte(coinKey))
	if coinIdx == -1 {
		return errors.New("missing coin array in response")
	}
	coinData := data[listIdx+coinIdx:]

	// Count non-zero balances first
	balanceCount := c.countWalletBalances(coinData)
	if balanceCount == 0 {
		log().Debug().Int("accountID", accountID).Msg("No non-zero balances found in wallet")
		return nil
	}

	// Calculate size and allocate buffer
	size := uint64(msgbus.SizeOfReqBalanceSnapshotHeader) + uint64(balanceCount)*uint64(msgbus.SizeOfBalance)
	offset, buf := c.msgBus.Allocate(size)

	// Write header directly to buffer
	pos := 0
	// AccountID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(accountID))
	pos += 8
	// BalancesLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(balanceCount))
	pos += 4
	// Padding (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Parse balances directly into buffer
	c.parseWalletBalancesInto(coinData, buf[pos:], balanceCount)

	// Publish to event bus
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventReqBalanceSnapshot,
		Index:  offset,
		Length: size,
	})

	log().Debug().Int("accountID", accountID).Int("balanceCount", balanceCount).Msg("Published balance snapshot")
	return nil
}

// countWalletBalances counts non-zero balances in the coin array
func (c *BybitHTTPClient) countWalletBalances(data []byte) int {
	count := 0
	curr := 0

	// Find start of coin array [
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return 0
	}
	curr++ // skip [

	// Iterate through each coin object
	depth := 0
	for curr < len(data) {
		b := data[curr]
		if b == '{' {
			depth++
			if depth == 1 {
				// Found a coin object, check if it has non-zero balance
				endIdx := c.findObjectEnd(data[curr:])
				if endIdx > 0 {
					coinObj := data[curr : curr+endIdx+1]
					if c.hasNonZeroBalance(coinObj) {
						count++
					}
				}
			}
		} else if b == '}' {
			depth--
		} else if b == ']' && depth == 0 {
			break
		}
		curr++
	}

	return count
}

// findObjectEnd finds the closing } for an object starting at data[0]
func (c *BybitHTTPClient) findObjectEnd(data []byte) int {
	depth := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '{' {
			depth++
		} else if data[i] == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// hasNonZeroBalance checks if a coin object has non-zero walletBalance
func (c *BybitHTTPClient) hasNonZeroBalance(coinObj []byte) bool {
	// Find walletBalance field
	const walletBalKey = "\"walletBalance\""
	idx := bytes.Index(coinObj, []byte(walletBalKey))
	if idx == -1 {
		return false
	}

	// Find the value (it's a string)
	idx += len(walletBalKey)
	for idx < len(coinObj) && coinObj[idx] != '"' {
		idx++
	}
	idx++ // skip opening quote

	start := idx
	for idx < len(coinObj) && coinObj[idx] != '"' {
		idx++
	}

	// Parse the balance value
	balance := parseFloat64(coinObj[start:idx])
	return balance > 0
}

// parseWalletBalancesInto parses wallet balances directly into buffer
func (c *BybitHTTPClient) parseWalletBalancesInto(data []byte, buf []byte, expectedCount int) {
	curr := 0
	pos := 0
	parsed := 0

	// Find start of coin array [
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return
	}
	curr++ // skip [

	// Iterate through each coin object
	depth := 0
	for curr < len(data) && parsed < expectedCount {
		b := data[curr]
		if b == '{' {
			depth++
			if depth == 1 {
				// Found a coin object
				endIdx := c.findObjectEnd(data[curr:])
				if endIdx > 0 {
					coinObj := data[curr : curr+endIdx+1]
					if c.hasNonZeroBalance(coinObj) {
						// Parse and write balance to buffer
						balance := c.parseCoinBalance(coinObj)
						// Write Balance struct directly (40 bytes)
						balancePtr := (*event.Balance)(unsafe.Pointer(&buf[pos]))
						balancePtr.TokenID = balance.TokenID
						balancePtr.Available = balance.Available
						balancePtr.Locked = balance.Locked
						balancePtr.Total = balance.Total
						pos += msgbus.SizeOfBalance
						parsed++
					}
				}
			}
		} else if b == '}' {
			depth--
		} else if b == ']' && depth == 0 {
			break
		}
		curr++
	}
}

// parseCoinBalance parses a single coin object into a Balance struct
func (c *BybitHTTPClient) parseCoinBalance(coinObj []byte) event.Balance {
	var balance event.Balance

	// Parse walletBalance (total)
	balance.Total = c.parseStringField(coinObj, "\"walletBalance\"")

	// Parse availableToWithdraw (available)
	balance.Available = c.parseStringField(coinObj, "\"availableToWithdraw\"")

	// Parse locked
	// Note: Bybit may not have a direct "locked" field, calculate from total - available
	locked := c.parseStringField(coinObj, "\"locked\"")
	if locked > 0 {
		balance.Locked = locked
	} else {
		balance.Locked = balance.Total - balance.Available
		if balance.Locked < 0 {
			balance.Locked = 0
		}
	}

	// TokenID would need to be looked up from catalog - for now use 0
	// A proper implementation would map coin name to TokenID
	balance.TokenID = 0

	return balance
}

// parseStringField parses a string field value as float64
func (c *BybitHTTPClient) parseStringField(data []byte, key string) float64 {
	idx := bytes.Index(data, []byte(key))
	if idx == -1 {
		return 0
	}

	idx += len(key)
	// Find colon and opening quote
	for idx < len(data) && data[idx] != '"' {
		idx++
	}
	idx++ // skip opening quote

	start := idx
	for idx < len(data) && data[idx] != '"' {
		idx++
	}

	return parseFloat64(data[start:idx])
}
