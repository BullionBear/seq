package binance

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/buger/jsonparser"
	"github.com/lxzan/gws"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

const (
	// WebSocket connection settings
	wsReadBufferSize    = 64 * 1024 // 64KB read buffer
	wsReconnectInterval = 5 * time.Second
	wsPingInterval      = 30 * time.Second
	wsPingWait          = 10 * time.Second

	// Binance stream names
	streamTrade    = "trade"
	streamAggTrade = "aggTrade"
)

// Byte constants for allocation-free message dispatch (P2-2): event types and
// stream-name fragments are compared as []byte, never converted to string.
var (
	bytesEventDepthUpdate = []byte("depthUpdate")
	bytesEventTrade       = []byte("trade")
	bytesEventAggTrade    = []byte("aggTrade")
	bytesEventKline       = []byte("kline")
	bytesStreamDepth      = []byte("depth")
	bytesStreamTrade      = []byte("@trade")
	bytesStreamAggTrade   = []byte("@aggTrade")
	bytesStreamKline      = []byte("kline")
)

// symbolPrecision caches per-symbol tick multipliers so the per-message path
// needs neither a catalog lookup (which allocates) nor math.Pow.
type symbolPrecision struct {
	priceMul float64 // 10^PricePrecision
	sizeMul  float64 // 10^SizePrecision
}

func newSymbolPrecision(pricePrecision, sizePrecision int) symbolPrecision {
	return symbolPrecision{
		priceMul: math.Pow(10, float64(pricePrecision)),
		sizeMul:  math.Pow(10, float64(sizePrecision)),
	}
}

// BinanceSpotDataClient handles Binance spot market data via WebSocket and HTTP
// It provides a unified interface for both real-time streaming data (WebSocket)
// and on-demand requests (HTTP REST API)
type BinanceSpotDataClient struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus

	// HTTP client for REST API requests (depth snapshots, etc.)
	httpClient *BinanceHTTPClient

	// WebSocket connection
	conn     *gws.Conn
	connLock sync.RWMutex

	// Subscription management
	depthSubs map[int]*DepthSubscriptionOptions // symbolID -> options
	tradeSubs map[int]*TradeSubscriptionOptions // symbolID -> options
	klineSubs map[int]common.Interval           // symbolID -> interval
	subsLock  sync.RWMutex

	// Stream to symbolID mapping for fast lookup during message processing
	streamToSymbol map[string]int // "btcusdt@depth@100ms" -> symbolID
	// Symbol name as sent by Binance ("BTCUSDT") -> symbolID, for
	// single-stream dispatch without building stream-name strings.
	symbolToID map[string]int
	// symbolID -> cached tick multipliers for the per-message parse path.
	precisions    map[int]symbolPrecision
	streamMapLock sync.RWMutex // guards streamToSymbol, symbolToID, precisions

	// Scratch price-level buffers reused across depth messages (grow-only,
	// high-water sized). Safe without locking: a client owns one WebSocket
	// connection and gws delivers its frames sequentially on the ReadLoop
	// goroutine (ParallelEnabled is off).
	scratchBids []common.PriceLevel
	scratchAsks []common.PriceLevel

	// Connection state
	connected  atomic.Bool
	shouldStop atomic.Bool

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building subscription messages
	msgBuffer bytes.Buffer
}

// NewBinanceSpotDataClient creates a new Binance spot data client
// It initializes both the WebSocket client for real-time data and the HTTP client for REST API requests
func NewBinanceSpotDataClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) *BinanceSpotDataClient {
	httpClient := NewBinanceHTTPClient(catalog, msgBus)
	return &BinanceSpotDataClient{
		catalog:        catalog,
		msgBus:         msgBus,
		httpClient:     &httpClient,
		depthSubs:      make(map[int]*DepthSubscriptionOptions, 64),
		tradeSubs:      make(map[int]*TradeSubscriptionOptions, 64),
		klineSubs:      make(map[int]common.Interval, 64),
		streamToSymbol: make(map[string]int, 128),
		symbolToID:     make(map[string]int, 128),
		precisions:     make(map[int]symbolPrecision, 128),
	}
}

// HasSub returns true if there are any subscriptions configured
func (c *BinanceSpotDataClient) HasSub() bool {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()
	return len(c.depthSubs) > 0 || len(c.tradeSubs) > 0 || len(c.klineSubs) > 0
}

// SubscribeDepthUpdate subscribes to a depth stream for a symbol.
// depthLevel 5/10/20 selects Binance partial-book streams (@depthN@… → DepthSnapshot);
// any other value selects diff depth (@depth@… → DepthUpdate).
// pushRateMs selects the push rate: >=1000 uses 1s, otherwise 100ms.
func (c *BinanceSpotDataClient) SubscribeDepthUpdate(symbolID int, depthLevel int, pushRateMs int) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	pushRate := PushRate100ms
	if pushRateMs >= 1000 {
		pushRate = PushRate1s
	}
	levels := 0
	switch depthLevel {
	case 5, 10, 20:
		levels = depthLevel
	}
	opts := &DepthSubscriptionOptions{PushRate: pushRate, Levels: levels}
	c.depthSubs[symbolID] = opts

	if c.connected.Load() {
		c.subscribeToDepthStream(symbolID, opts)
	}
}

// SubscribeTrade subscribes to trade stream for a symbol.
// useAggTrade selects between regular trade and aggTrade streams.
func (c *BinanceSpotDataClient) SubscribeTrade(symbolID int, useAggTrade bool) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	opts := &TradeSubscriptionOptions{UseAggTrade: useAggTrade}
	c.tradeSubs[symbolID] = opts

	if c.connected.Load() {
		stream := streamTrade
		if useAggTrade {
			stream = streamAggTrade
		}
		c.subscribeToStream(symbolID, stream)
	}
}

// SubscribeKline subscribes to a kline stream for a symbol.
// interval is a Binance-style string (e.g. "1m", "1h", "1d").
func (c *BinanceSpotDataClient) SubscribeKline(symbolID int, interval string) {
	iv, err := common.ParseInterval(interval)
	if err != nil || iv == common.IntervalUnknown {
		log().Error().Str("interval", interval).Msg("Invalid kline interval for Binance")
		return
	}

	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	c.klineSubs[symbolID] = iv

	if c.connected.Load() {
		c.subscribeToKlineStream(symbolID, iv)
	}
}

// ReqDepthSnapshot requests a depth snapshot via HTTP REST API.
func (c *BinanceSpotDataClient) ReqDepthSnapshot(symbolID int, limit int) error {
	return c.httpClient.ReqDepthSnapshot(symbolID, limit)
}

// ReqHistoricalKline requests historical klines via HTTP REST API.
func (c *BinanceSpotDataClient) ReqHistoricalKline(symbolID int, interval string, startTimeNs, endTimeNs uint64, limit int) error {
	return c.httpClient.ReqHistoricalKline(symbolID, interval, startTimeNs, endTimeNs, limit)
}

// Connect establishes the WebSocket connection and starts processing
func (c *BinanceSpotDataClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	// Build combined stream URL
	streams := c.buildStreamList()
	if len(streams) == 0 {
		log().Warn().Msg("No subscriptions configured, skipping Binance connection")
		return nil
	}

	url := c.buildStreamURL(streams)
	log().Info().Str("url", url).Int("streams", len(streams)).Msg("Connecting to Binance WebSocket")

	// Create event handler
	handler := &wsEventHandler{client: c}

	// Create client options with optimized settings
	option := &gws.ClientOption{
		Addr:             url,
		ReadBufferSize:   wsReadBufferSize,
		CheckUtf8Enabled: false, // Disable UTF-8 check for performance
		NewDialer: func() (gws.Dialer, error) {
			return &ipv4Dialer{}, nil
		},
	}

	// Create WebSocket client using gws.NewClient
	conn, _, err := gws.NewClient(handler, option)
	if err != nil {
		return err
	}

	c.connLock.Lock()
	c.conn = conn
	c.connLock.Unlock()
	c.connected.Store(true)

	// Start ReadLoop in a goroutine (this is blocking)
	go func() {
		conn.ReadLoop()
		// ReadLoop returned, connection closed
		c.handleDisconnect()
	}()

	// Start ping loop to keep connection alive
	go c.pingLoop()

	log().Info().Msg("Connected to Binance WebSocket")
	return nil
}

// Disconnect closes the WebSocket connection
func (c *BinanceSpotDataClient) Disconnect() {
	c.shouldStop.Store(true)
	if c.cancel != nil {
		c.cancel()
	}

	c.connLock.Lock()
	if c.conn != nil {
		_ = c.conn.WriteClose(1000, nil)
		c.conn = nil
	}
	c.connLock.Unlock()
	c.connected.Store(false)

	log().Info().Msg("Disconnected from Binance WebSocket")
}

// buildStreamList builds the list of streams to subscribe
func (c *BinanceSpotDataClient) buildStreamList() []string {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()

	streams := make([]string, 0, len(c.depthSubs)+len(c.tradeSubs)+len(c.klineSubs))

	for symbolID, opts := range c.depthSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
			continue
		}
		streamName := strings.ToLower(symbol.Name) + "@" + opts.PushRate.StreamSuffix(opts.Levels)
		streams = append(streams, streamName)
		c.registerStream(streamName, symbol)
	}

	for symbolID, opts := range c.tradeSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for trade subscription")
			continue
		}
		stream := streamTrade
		if opts != nil && opts.UseAggTrade {
			stream = streamAggTrade
		}
		streamName := strings.ToLower(symbol.Name) + "@" + stream
		streams = append(streams, streamName)
		c.registerStream(streamName, symbol)
	}

	for symbolID, iv := range c.klineSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for kline subscription")
			continue
		}
		streamName := strings.ToLower(symbol.Name) + "@kline_" + iv.BinanceStream()
		streams = append(streams, streamName)
		c.registerStream(streamName, symbol)
	}

	return streams
}

// registerStream records the stream -> symbol mapping plus the lookups the
// hot parse path needs: symbol name -> ID (single-stream dispatch) and
// symbolID -> tick multipliers.
func (c *BinanceSpotDataClient) registerStream(streamName string, symbol *catalog.Symbol) {
	c.streamMapLock.Lock()
	c.streamToSymbol[streamName] = symbol.ID
	c.symbolToID[symbol.Name] = symbol.ID
	c.precisions[symbol.ID] = newSymbolPrecision(symbol.PricePrecision, symbol.SizePrecision)
	c.streamMapLock.Unlock()
}

// buildStreamURL builds the combined stream WebSocket URL
func (c *BinanceSpotDataClient) buildStreamURL(streams []string) string {
	if len(streams) == 0 {
		return BaseWsURL + "/ws"
	}
	if len(streams) == 1 {
		return BaseWsURL + "/ws/" + streams[0]
	}
	return BaseWsURL + "/stream?streams=" + strings.Join(streams, "/")
}

// subscribeToStream sends a subscription message for a specific stream
func (c *BinanceSpotDataClient) subscribeToStream(symbolID int, streamType string) {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for subscription")
		return
	}

	streamName := strings.ToLower(symbol.Name) + "@" + streamType
	c.registerStream(streamName, symbol)
	c.sendSubscription(streamName)
}

// subscribeToKlineStream sends a subscription message for a kline stream.
func (c *BinanceSpotDataClient) subscribeToKlineStream(symbolID int, iv common.Interval) {
	c.subscribeToStream(symbolID, "kline_"+iv.BinanceStream())
}

// subscribeToDepthStream sends a subscription message for a depth stream.
func (c *BinanceSpotDataClient) subscribeToDepthStream(symbolID int, opts *DepthSubscriptionOptions) {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
		return
	}

	levels := 0
	pushRate := PushRate100ms
	if opts != nil {
		levels = opts.Levels
		pushRate = opts.PushRate
	}
	streamName := strings.ToLower(symbol.Name) + "@" + pushRate.StreamSuffix(levels)
	c.registerStream(streamName, symbol)
	c.sendSubscription(streamName)
}

// sendSubscription sends a subscription message for a stream name
// (the stream must already be registered via registerStream).
func (c *BinanceSpotDataClient) sendSubscription(streamName string) {
	// Build subscription message
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"method":"SUBSCRIBE","params":["`)
	c.msgBuffer.WriteString(streamName)
	c.msgBuffer.WriteString(`"],"id":`)
	c.msgBuffer.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	c.msgBuffer.WriteByte('}')

	c.connLock.RLock()
	conn := c.conn
	c.connLock.RUnlock()

	if conn != nil {
		if err := conn.WriteMessage(gws.OpcodeText, c.msgBuffer.Bytes()); err != nil {
			log().Error().Err(err).Str("stream", streamName).Msg("Failed to send subscription message")
		}
	}
}

// pingLoop sends periodic pings to keep the connection alive
func (c *BinanceSpotDataClient) pingLoop() {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.connLock.RLock()
			conn := c.conn
			c.connLock.RUnlock()

			if conn != nil {
				if err := conn.WritePing(nil); err != nil {
					log().Warn().Err(err).Msg("Failed to send ping")
				}
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *BinanceSpotDataClient) handleDisconnect() {
	c.connected.Store(false)

	if c.shouldStop.Load() {
		return
	}

	log().Warn().Msg("WebSocket disconnected, attempting to reconnect...")

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.Connect(c.ctx); err != nil {
			log().Error().Err(err).Msg("Reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes a raw WebSocket message using jsonparser.
//
// P2-2 contract: all field extraction stays on []byte subslices of the
// connection read buffer — no string conversions, no per-message
// allocations. Symbol resolution goes through map[string]T lookups keyed
// with string(b) (compiler-optimized, non-allocating); only integer IDs
// cross into the arena. Nothing derived from data may be retained past
// this call (P2-1 buffer lifetime contract).
func (c *BinanceSpotDataClient) processMessage(data []byte) {
	// Check if this is a combined stream message (has "stream" field)
	stream, _, _, err := jsonparser.Get(data, "stream")
	if err == nil {
		// Combined stream format: {"stream":"...","data":{...}}
		msgData, _, _, err := jsonparser.Get(data, "data")
		if err != nil {
			// Per-message path: Debug only (P2-3); disabled at production level.
			log().Debug().Err(err).Msg("Failed to get data field from combined stream message")
			return
		}
		c.processStreamMessage(stream, msgData)
		return
	}

	// Single stream format - determine type from event field
	eventType, _, _, err := jsonparser.Get(data, "e")
	if err != nil {
		// Might be a subscription response or error
		return
	}

	// For single stream, resolve the symbol name ("s", uppercase) directly
	// to a symbolID — no stream-name construction.
	symbol, _, _, _ := jsonparser.Get(data, "s")
	if len(symbol) == 0 {
		return
	}
	c.streamMapLock.RLock()
	symbolID, ok := c.symbolToID[string(symbol)]
	c.streamMapLock.RUnlock()
	if !ok {
		return // not a subscribed symbol
	}

	switch {
	case bytes.Equal(eventType, bytesEventDepthUpdate):
		c.processDepthUpdate(symbolID, data)
	case bytes.Equal(eventType, bytesEventTrade), bytes.Equal(eventType, bytesEventAggTrade):
		c.processTrade(symbolID, data)
	case bytes.Equal(eventType, bytesEventKline):
		c.processKline(symbolID, data)
	}
}

// processStreamMessage routes a combined-stream message to the appropriate handler
func (c *BinanceSpotDataClient) processStreamMessage(stream, data []byte) {
	c.streamMapLock.RLock()
	symbolID, ok := c.streamToSymbol[string(stream)]
	c.streamMapLock.RUnlock()

	if !ok {
		return // unknown stream (cold: only after a subscription bug)
	}

	// Determine message type from stream name
	if bytes.Contains(stream, bytesStreamKline) {
		c.processKline(symbolID, data)
	} else if bytes.Contains(stream, bytesStreamDepth) {
		if isPartialBookStream(stream) {
			c.processDepthSnapshot(symbolID, data)
		} else {
			c.processDepthUpdate(symbolID, data)
		}
	} else if isTradeStream(stream) {
		c.processTrade(symbolID, data)
	}
}

// isTradeStream reports whether stream is a Binance trade or aggregate-trade
// name. Suffix match is required: bytes.Contains(..., "trade") misses
// "@aggTrade" (capital T) and would be ambiguous if other streams ever
// embedded the substring.
func isTradeStream(stream []byte) bool {
	return bytes.HasSuffix(stream, bytesStreamTrade) || bytes.HasSuffix(stream, bytesStreamAggTrade)
}

// isPartialBookStream reports whether stream is a Binance partial-book name
// (e.g. "btcusdt@depth5@100ms"), as opposed to diff depth ("btcusdt@depth@100ms").
func isPartialBookStream(stream []byte) bool {
	idx := bytes.Index(stream, []byte("@depth"))
	if idx < 0 {
		return false
	}
	rest := stream[idx+len("@depth"):]
	return len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9'
}

// getPrecision returns the cached tick multipliers for a symbol. On a miss
// (stream registered without going through registerStream, e.g. in tests)
// it falls back to the catalog once and caches the result — the hot path
// afterwards is a read-locked map hit.
func (c *BinanceSpotDataClient) getPrecision(symbolID int) (symbolPrecision, bool) {
	c.streamMapLock.RLock()
	prec, ok := c.precisions[symbolID]
	c.streamMapLock.RUnlock()
	if ok {
		return prec, true
	}

	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		// Per-message path: Debug only (P2-3); disabled at production level.
		log().Debug().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth update")
		return symbolPrecision{}, false
	}
	prec = newSymbolPrecision(symbol.PricePrecision, symbol.SizePrecision)
	c.streamMapLock.Lock()
	c.precisions[symbolID] = prec
	c.streamMapLock.Unlock()
	return prec, true
}

// processDepthSnapshot parses and publishes a partial-book depth snapshot.
// Binance partial book format (no "e" field):
//
//	{
//	  "lastUpdateId": 160,
//	  "bids": [["price","qty"],...],
//	  "asks": [["price","qty"],...]
//	}
func (c *BinanceSpotDataClient) processDepthSnapshot(symbolID int, data []byte) {
	prec, ok := c.getPrecision(symbolID)
	if !ok {
		return
	}

	lastUpdateID, err := jsonparser.GetInt(data, "lastUpdateId")
	if err != nil {
		return
	}

	c.scratchBids = appendPriceLevels(c.scratchBids[:0], data, "bids", prec)
	c.scratchAsks = appendPriceLevels(c.scratchAsks[:0], data, "asks", prec)

	snapshot := event.DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   int(lastUpdateID),
		Timestamp: uint64(time.Now().UnixNano()),
		Asks:      c.scratchAsks,
		Bids:      c.scratchBids,
	}

	size := uint64(snapshot.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventDepthSnapshot, size)
	if !ok {
		return // dropped under overflow; next partial push resyncs the top-N book
	}
	if err := snapshot.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return
	}
	c.msgBus.Publish(ref)
}

// processDepthUpdate parses and publishes a depth update event
// Binance depth update format:
//
//	{
//	  "e": "depthUpdate",
//	  "E": 123456789,     // Event time
//	  "s": "BTCUSDT",     // Symbol
//	  "U": 157,           // First update ID in event
//	  "u": 160,           // Final update ID in event
//	  "b": [["price","qty"],...],  // Bids
//	  "a": [["price","qty"],...]   // Asks
//	}
func (c *BinanceSpotDataClient) processDepthUpdate(symbolID int, data []byte) {
	prec, ok := c.getPrecision(symbolID)
	if !ok {
		return
	}

	var depthUpdate event.DepthUpdate
	depthUpdate.SymbolID = symbolID

	// Parse event time
	eventTime, _ := jsonparser.GetInt(data, "E")
	depthUpdate.Timestamp = uint64(eventTime) * 1_000_000 // Convert ms to ns

	// Parse update IDs
	firstUpdateID, _ := jsonparser.GetInt(data, "U")
	finalUpdateID, _ := jsonparser.GetInt(data, "u")
	depthUpdate.PreviousDepthID = int(firstUpdateID) - 1
	depthUpdate.DepthID = int(firstUpdateID)
	depthUpdate.CurrentDepthID = int(finalUpdateID)
	depthUpdate.NextDepthID = int(finalUpdateID) + 1

	// Parse bids and asks into the reusable scratch buffers. The slices are
	// only referenced until Encode below copies the levels into the arena.
	c.scratchBids = appendPriceLevels(c.scratchBids[:0], data, "b", prec)
	c.scratchAsks = appendPriceLevels(c.scratchAsks[:0], data, "a", prec)
	depthUpdate.Bids = c.scratchBids
	depthUpdate.Asks = c.scratchAsks

	// Publish to event bus
	size := uint64(depthUpdate.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventDepthUpdate, size)
	if !ok {
		return // dropped under overflow; orderbook re-syncs via DepthID gap
	}
	if err := depthUpdate.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return
	}
	c.msgBus.Publish(ref)
}

// processKline parses and publishes a kline event.
// Binance kline format:
//
//	{
//	  "e": "kline",
//	  "E": 123456789,
//	  "s": "BNBBTC",
//	  "k": {
//	    "t": 123400000, "T": 123499999, "i": "1m",
//	    "o": "0.0010", "c": "0.0020", "h": "0.0025", "l": "0.0015",
//	    "v": "1000", "q": "1.0000", "n": 100, "x": false
//	  }
//	}
func (c *BinanceSpotDataClient) processKline(symbolID int, data []byte) {
	kobj, _, _, err := jsonparser.Get(data, "k")
	if err != nil {
		return
	}

	intervalBytes, _, _, _ := jsonparser.Get(kobj, "i")
	iv, err := common.ParseInterval(string(intervalBytes))
	if err != nil {
		return
	}

	startMs, _ := jsonparser.GetInt(kobj, "t")
	endMs, _ := jsonparser.GetInt(kobj, "T")
	eventTime, _ := jsonparser.GetInt(data, "E")
	tradeCount, _ := jsonparser.GetInt(kobj, "n")
	closed, _ := jsonparser.GetBoolean(kobj, "x")

	openBytes, _, _, _ := jsonparser.Get(kobj, "o")
	highBytes, _, _, _ := jsonparser.Get(kobj, "h")
	lowBytes, _, _, _ := jsonparser.Get(kobj, "l")
	closeBytes, _, _, _ := jsonparser.Get(kobj, "c")
	volBytes, _, _, _ := jsonparser.Get(kobj, "v")
	quoteBytes, _, _, _ := jsonparser.Get(kobj, "q")

	kline := event.Kline{
		SymbolID:    symbolID,
		Interval:    iv,
		StartTime:   uint64(startMs) * 1_000_000,
		EndTime:     uint64(endMs) * 1_000_000,
		Timestamp:   uint64(eventTime) * 1_000_000,
		Open:        parseFloat64(openBytes),
		High:        parseFloat64(highBytes),
		Low:         parseFloat64(lowBytes),
		Close:       parseFloat64(closeBytes),
		Volume:      parseFloat64(volBytes),
		QuoteVolume: parseFloat64(quoteBytes),
		TradeCount:  int(tradeCount),
		Closed:      closed,
	}

	size := uint64(kline.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventKline, size)
	if !ok {
		return // dropped under overflow
	}
	if err := kline.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return
	}
	c.msgBus.Publish(ref)
}

// processTrade parses and publishes a trade or aggregate-trade as Tick.
// Shared fields (p/q/T/m) are identical on @trade and @aggTrade; trade ID
// fields differ (t vs a) and are not part of Tick.
//
//	trade:    {"e":"trade","p":"...","q":"...","T":...,"m":true,...}
//	aggTrade: {"e":"aggTrade","p":"...","q":"...","T":...,"m":true,...}
func (c *BinanceSpotDataClient) processTrade(symbolID int, data []byte) {
	var tick event.Tick
	tick.SymbolID = symbolID

	// Parse trade time
	tradeTime, _ := jsonparser.GetInt(data, "T")
	tick.Timestamp = uint64(tradeTime) * 1_000_000 // Convert ms to ns

	// Parse price - directly from bytes without string allocation
	priceBytes, _, _, _ := jsonparser.Get(data, "p")
	tick.Price = parseFloat64(priceBytes)

	// Parse quantity
	qtyBytes, _, _, _ := jsonparser.Get(data, "q")
	tick.Qty = parseFloat64(qtyBytes)

	// Parse side: m=true means buyer is maker, so it's a sell; m=false means buy
	isBuyerMaker, _ := jsonparser.GetBoolean(data, "m")
	if isBuyerMaker {
		tick.Side = common.SideSell
	} else {
		tick.Side = common.SideBuy
	}

	// Publish to event bus
	size := uint64(tick.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventTick, size)
	if !ok {
		return // dropped under overflow
	}
	if err := tick.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return
	}
	c.msgBus.Publish(ref)
}

// appendPriceLevels parses an array of [price, qty] arrays from JSON and
// appends the levels to dst (single pass, amortized zero allocation once the
// caller's scratch buffer has reached its high-water capacity).
func appendPriceLevels(dst []common.PriceLevel, data []byte, key string, prec symbolPrecision) []common.PriceLevel {
	_, _ = jsonparser.ArrayEach(data, func(value []byte, _ jsonparser.ValueType, _ int, _ error) {
		var pl common.PriceLevel
		elemIdx := 0
		_, _ = jsonparser.ArrayEach(value, func(elem []byte, _ jsonparser.ValueType, _ int, _ error) {
			// Remove quotes if present
			if len(elem) >= 2 && elem[0] == '"' {
				elem = elem[1 : len(elem)-1]
			}
			if elemIdx == 0 {
				pl.Price = parseFloat64(elem)
			} else if elemIdx == 1 {
				pl.Quantity = parseFloat64(elem)
			}
			elemIdx++
		})
		pl.PriceTick = int(math.Round(pl.Price * prec.priceMul))
		pl.QuantityTick = int(math.Round(pl.Quantity * prec.sizeMul))
		dst = append(dst, pl)
	}, key)
	return dst
}

// parseFloat64 parses a float64 from bytes without string allocation
func parseFloat64(b []byte) float64 {
	// Remove quotes if present (jsonparser might include them)
	if len(b) >= 2 && b[0] == '"' {
		b = b[1 : len(b)-1]
	}

	if len(b) == 0 {
		return 0
	}

	var result float64
	var decimalPlace float64
	negative := false
	hasDecimal := false

	i := 0
	if b[0] == '-' {
		negative = true
		i = 1
	} else if b[0] == '+' {
		i = 1
	}

	for ; i < len(b); i++ {
		ch := b[i]
		if ch == '.' {
			hasDecimal = true
			decimalPlace = 0.1
			continue
		}
		if ch < '0' || ch > '9' {
			break
		}
		digit := float64(ch - '0')
		if hasDecimal {
			result += digit * decimalPlace
			decimalPlace *= 0.1
		} else {
			result = result*10 + digit
		}
	}

	if negative {
		result = -result
	}

	return result
}

// wsEventHandler implements gws.Event interface for WebSocket callbacks
type wsEventHandler struct {
	client *BinanceSpotDataClient
}

func (h *wsEventHandler) OnOpen(socket *gws.Conn) {
	log().Info().Msg("Binance WebSocket connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
}

func (h *wsEventHandler) OnClose(socket *gws.Conn, err error) {
	log().Info().Err(err).Msg("Binance WebSocket connection closed")
}

func (h *wsEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsEventHandler) OnPong(socket *gws.Conn, payload []byte) {
	// Pong received, connection is alive
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
}

// OnMessage handles one WebSocket frame.
//
// Buffer lifetime contract (P2-1): message.Bytes() aliases a pooled read
// buffer owned by gws; message.Close() recycles it into the pool, so frame
// reads allocate nothing in steady state. The contents are valid only until
// this handler returns — parsing (processMessage) and the arena encode must
// complete synchronously here, and no subslice of the payload may be
// retained. Frames arrive sequentially per connection (gws ParallelEnabled
// is off), which is what makes the client's scratch buffers safe.
func (h *wsEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	// Reset deadline on message received
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	// Process the message
	h.client.processMessage(message.Bytes())
}
