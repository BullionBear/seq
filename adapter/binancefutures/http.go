package binancefutures

import (
	"bytes"
	"errors"
	"strconv"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/buger/jsonparser"

	"github.com/valyala/fasthttp"
)

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// errSnapshotDropped is returned when a depth snapshot could not be published
// because the event bus dropped it under overflow; the caller may retry.
var errSnapshotDropped = errors.New("binancefutures: depth snapshot dropped under event bus overflow")

// errKlineDropped is returned when a historical kline response could not be published.
var errKlineDropped = errors.New("binancefutures: historical kline dropped under event bus overflow")

type BinanceFuturesHTTPClient struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	client  fasthttp.Client
	buffer  bytes.Buffer
	baseURL string
}

func NewBinanceFuturesHTTPClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) BinanceFuturesHTTPClient {
	return BinanceFuturesHTTPClient{
		catalog: catalog,
		msgBus:  msgBus,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
		baseURL: BaseURL,
	}
}

// ReqDepthSnapshot fetches order book depth from Binance USD-M futures REST API.
func (c *BinanceFuturesHTTPClient) ReqDepthSnapshot(symbolId int, limit int) error {
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

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURIBytes(c.buffer.Bytes())
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	err = c.client.Do(req, resp)
	if err != nil {
		return err
	}

	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		return &HTTPError{
			StatusCode: statusCode,
			Body:       string(resp.Body()),
		}
	}

	jsonData := resp.Body()

	depthID, askCount, bidCount, err := c.countDepthLevels(jsonData)
	if err != nil {
		return err
	}

	size := uint64(event.DepthSnapshotHeaderSize) +
		uint64(askCount)*uint64(event.PriceLevelSize) +
		uint64(bidCount)*uint64(event.PriceLevelSize)

	ref, buf, ok := c.msgBus.Allocate(event.TopicEventDepthSnapshot, size)
	if !ok {
		return errSnapshotDropped
	}

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

	err = c.parsePriceLevelsInto(jsonData, "\"asks\"", asks, symbol.PricePrecision, symbol.SizePrecision)
	if err != nil {
		c.msgBus.Cancel(ref)
		return err
	}
	err = c.parsePriceLevelsInto(jsonData, "\"bids\"", bids, symbol.PricePrecision, symbol.SizePrecision)
	if err != nil {
		c.msgBus.Cancel(ref)
		return err
	}

	timestamp := uint64(time.Now().UnixNano())
	snapshot := event.DepthSnapshot{
		SymbolID:  symbolId,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}
	if err := snapshot.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return err
	}

	c.msgBus.Publish(ref)
	return nil
}

func (c *BinanceFuturesHTTPClient) countDepthLevels(data []byte) (depthID, askCount, bidCount int, err error) {
	const lastUpdateIdKey = "\"lastUpdateId\""
	idx := bytes.Index(data, []byte(lastUpdateIdKey))
	if idx == -1 {
		return 0, 0, 0, errors.New("missing lastUpdateId")
	}

	idx += len(lastUpdateIdKey)
	for idx < len(data) && data[idx] != ':' {
		idx++
	}
	idx++

	for idx < len(data) && (data[idx] == ' ' || data[idx] == '\n' || data[idx] == '\t') {
		idx++
	}

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

	askCount = c.countArrayElements(data, "\"asks\"")
	bidCount = c.countArrayElements(data, "\"bids\"")

	return depthID, askCount, bidCount, nil
}

func (c *BinanceFuturesHTTPClient) countArrayElements(data []byte, key string) int {
	keyIdx := bytes.Index(data, []byte(key))
	if keyIdx == -1 {
		return 0
	}

	curr := keyIdx + len(key)
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return 0
	}
	curr++

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
				break
			}
		}
		curr++
	}

	return count
}

func (c *BinanceFuturesHTTPClient) parsePriceLevelsInto(data []byte, key string, levels []common.PriceLevel, pricePrecision, sizePrecision int) error {
	if len(levels) == 0 {
		return nil
	}

	keyIdx := bytes.Index(data, []byte(key))
	if keyIdx == -1 {
		return nil
	}

	curr := keyIdx + len(key)
	for curr < len(data) && data[curr] != '[' {
		curr++
	}
	if curr >= len(data) {
		return errors.New("array start not found for " + key)
	}
	curr++

	for itemIdx := 0; itemIdx < len(levels); itemIdx++ {
		for curr < len(data) && data[curr] != '[' {
			curr++
		}
		curr++

		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		curr++
		pStart := curr
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		price, err := strconv.ParseFloat(unsafeString(data[pStart:curr]), 64)
		if err != nil {
			return err
		}
		curr++

		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		curr++
		qStart := curr
		for curr < len(data) && data[curr] != '"' {
			curr++
		}
		qty, err := strconv.ParseFloat(unsafeString(data[qStart:curr]), 64)
		if err != nil {
			return err
		}
		curr++

		levels[itemIdx].Price = price
		levels[itemIdx].Quantity = qty
		levels[itemIdx].PriceTick = common.PriceToTick(price, pricePrecision)
		levels[itemIdx].QuantityTick = common.QuantityToTick(qty, sizePrecision)

		for curr < len(data) && data[curr] != ']' {
			curr++
		}
		curr++
	}

	return nil
}

// ReqHistoricalKline fetches historical klines via GET /fapi/v1/klines and
// publishes TopicEventRespHistoricalKline. Times are nanoseconds; 0 omits the bound.
func (c *BinanceFuturesHTTPClient) ReqHistoricalKline(symbolID int, interval string, startTimeNs, endTimeNs uint64, limit int) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	iv, err := common.ParseInterval(interval)
	if err != nil {
		return err
	}

	c.buffer.Reset()
	c.buffer.Grow(len(c.baseURL) + len(EndpointKlines) + len(symbol.Name) + 96)
	c.buffer.WriteString(c.baseURL)
	c.buffer.WriteString(EndpointKlines)
	c.buffer.WriteByte('?')
	c.buffer.WriteString("symbol=")
	c.buffer.WriteString(symbol.Name)
	c.buffer.WriteString("&interval=")
	c.buffer.WriteString(iv.BinanceStream())
	if startTimeNs > 0 {
		c.buffer.WriteString("&startTime=")
		c.buffer.WriteString(strconv.FormatUint(startTimeNs/1_000_000, 10))
	}
	if endTimeNs > 0 {
		c.buffer.WriteString("&endTime=")
		c.buffer.WriteString(strconv.FormatUint(endTimeNs/1_000_000, 10))
	}
	if limit > 0 {
		c.buffer.WriteString("&limit=")
		c.buffer.WriteString(strconv.Itoa(limit))
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURIBytes(c.buffer.Bytes())
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	if err = c.client.Do(req, resp); err != nil {
		return err
	}
	if statusCode := resp.StatusCode(); statusCode != fasthttp.StatusOK {
		return &HTTPError{StatusCode: statusCode, Body: string(resp.Body())}
	}

	bars, err := parseBinanceKlines(resp.Body())
	if err != nil {
		return err
	}

	out := event.RespHistoricalKline{
		SymbolID: symbolID,
		Interval: iv,
		Bars:     bars,
	}
	size := uint64(out.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventRespHistoricalKline, size)
	if !ok {
		return errKlineDropped
	}
	if err := out.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return err
	}
	c.msgBus.Publish(ref)
	return nil
}

func parseBinanceKlines(data []byte) ([]event.KlineBar, error) {
	bars := make([]event.KlineBar, 0, 64)
	var parseErr error
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, _ error) {
		if parseErr != nil || dataType != jsonparser.Array {
			return
		}
		bar, err := parseBinanceKlineRow(value)
		if err != nil {
			parseErr = err
			return
		}
		bars = append(bars, bar)
	})
	if err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	return bars, nil
}

func parseBinanceKlineRow(row []byte) (event.KlineBar, error) {
	var bar event.KlineBar
	idx := 0
	_, err := jsonparser.ArrayEach(row, func(value []byte, dataType jsonparser.ValueType, _ int, _ error) {
		switch idx {
		case 0:
			if dataType == jsonparser.Number {
				if ms, e := jsonparser.ParseInt(value); e == nil {
					bar.StartTime = uint64(ms) * 1_000_000
					bar.Timestamp = bar.StartTime
				}
			}
		case 1:
			bar.Open = parseFloat64(value)
		case 2:
			bar.High = parseFloat64(value)
		case 3:
			bar.Low = parseFloat64(value)
		case 4:
			bar.Close = parseFloat64(value)
		case 5:
			bar.Volume = parseFloat64(value)
		case 6:
			if dataType == jsonparser.Number {
				if ms, e := jsonparser.ParseInt(value); e == nil {
					bar.EndTime = uint64(ms) * 1_000_000
				}
			}
		case 7:
			bar.QuoteVolume = parseFloat64(value)
		case 8:
			if dataType == jsonparser.Number {
				if n, e := jsonparser.ParseInt(value); e == nil {
					bar.TradeCount = int(n)
				}
			}
		}
		idx++
	})
	if err != nil {
		return event.KlineBar{}, err
	}
	bar.Closed = true
	return bar, nil
}
