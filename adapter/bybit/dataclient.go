package bybit

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
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
	wsPingInterval      = 20 * time.Second // Bybit recommends 20s
	wsPingWait          = 10 * time.Second
)

// Byte constants for allocation-free message dispatch (P2-2): ops, topic
// prefixes, and message types are compared as []byte, never converted to
// string.
var (
	bytesOpPong         = []byte("pong")
	bytesOpSubscribe    = []byte("subscribe")
	bytesTopicOrderbook = []byte("orderbook.")
	bytesTopicTrade     = []byte("publicTrade.")
	bytesTypeSnapshot   = []byte("snapshot")
	bytesTypeDelta      = []byte("delta")
	bytesSideSell       = []byte("Sell")
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

// BybitDataClient handles Bybit market data via WebSocket and HTTP
// It provides a unified interface for both real-time streaming data (WebSocket)
// and on-demand requests (HTTP REST API)
//
// Key difference from Binance: Bybit requires separate WebSocket connections
// for each channel type (spot, linear, inverse, option)
type BybitDataClient struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus

	// HTTP client for REST API requests (depth snapshots, etc.)
	httpClient *BybitHTTPClient

	// WebSocket connections per category
	// Bybit requires separate connections for spot, linear, inverse, option
	conns     map[Category]*wsConnection
	connsLock sync.RWMutex

	// Subscription management
	depthSubs map[int]*DepthSubscriptionOptions // symbolID -> options
	tradeSubs map[int]struct{}                  // symbolID -> exists
	subsLock  sync.RWMutex

	// Topic to symbolID mapping for fast lookup during message processing
	topicToSymbol map[string]int // "orderbook.50.BTCUSDT" -> symbolID
	// symbolID -> cached tick multipliers for the per-message parse path.
	precisions   map[int]symbolPrecision
	topicMapLock sync.RWMutex // guards topicToSymbol, precisions

	// Connection state
	shouldStop atomic.Bool

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building subscription messages
	msgBuffer bytes.Buffer
}

// wsConnection represents a single WebSocket connection for a category
type wsConnection struct {
	conn      *gws.Conn
	connLock  sync.RWMutex
	connected atomic.Bool
	category  Category
	client    *BybitDataClient

	// Scratch price-level buffers reused across depth messages (grow-only,
	// high-water sized). They are per-connection because one client owns
	// several category connections whose ReadLoop goroutines run
	// concurrently; within a connection frames are processed sequentially
	// (gws ParallelEnabled is off).
	scratchBids []common.PriceLevel
	scratchAsks []common.PriceLevel
}

// NewBybitDataClient creates a new Bybit data client
func NewBybitDataClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) *BybitDataClient {
	httpClient := NewBybitHTTPClient(catalog, msgBus)
	return &BybitDataClient{
		catalog:       catalog,
		msgBus:        msgBus,
		httpClient:    &httpClient,
		conns:         make(map[Category]*wsConnection, 4),
		depthSubs:     make(map[int]*DepthSubscriptionOptions, 64),
		tradeSubs:     make(map[int]struct{}, 64),
		topicToSymbol: make(map[string]int, 128),
		precisions:    make(map[int]symbolPrecision, 128),
	}
}

// HasSub returns true if there are any subscriptions configured
func (c *BybitDataClient) HasSub() bool {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()
	return len(c.depthSubs) > 0 || len(c.tradeSubs) > 0
}

// SubscribeDepthUpdate subscribes to depth update stream for a symbol.
// depthLevel maps to Bybit depth levels (1, 50, 200, 500); defaults to 50.
// pushRateMs is ignored (Bybit determines push rate from depth level).
func (c *BybitDataClient) SubscribeDepthUpdate(symbolID int, depthLevel int, pushRateMs int) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	depth := DepthLevel(depthLevel)
	if depth == 0 {
		depth = DepthLevel50
	}
	opts := &DepthSubscriptionOptions{Depth: depth}
	c.depthSubs[symbolID] = opts

	// If already connected, send subscribe message
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
		return
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		log().Error().Str("product", symbol.Product.Slug).Msg("Unsupported product type for Bybit")
		return
	}

	c.connsLock.RLock()
	wsConn, exists := c.conns[category]
	c.connsLock.RUnlock()

	if exists && wsConn.connected.Load() {
		c.subscribeToDepthStream(symbolID, opts.Depth, category)
	}
}

// SubscribeTrade subscribes to trade stream for a symbol.
// useAggTrade is ignored (Bybit has a single trade stream type).
func (c *BybitDataClient) SubscribeTrade(symbolID int, useAggTrade bool) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	c.tradeSubs[symbolID] = struct{}{}

	// If already connected, send subscribe message
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for trade subscription")
		return
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		log().Error().Str("product", symbol.Product.Slug).Msg("Unsupported product type for Bybit")
		return
	}

	c.connsLock.RLock()
	wsConn, exists := c.conns[category]
	c.connsLock.RUnlock()

	if exists && wsConn.connected.Load() {
		c.subscribeToTradeStream(symbolID, category)
	}
}

// ReqDepthSnapshot requests a depth snapshot via HTTP REST API
func (c *BybitDataClient) ReqDepthSnapshot(symbolID int, limit int) error {
	return c.httpClient.ReqDepthSnapshot(symbolID, limit)
}

// Connect establishes WebSocket connections and starts processing
func (c *BybitDataClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	// Group subscriptions by category
	categoryTopics := c.buildCategoryTopics()
	if len(categoryTopics) == 0 {
		log().Warn().Msg("No subscriptions configured, skipping Bybit connection")
		return nil
	}

	// Connect to each required category
	for category, topics := range categoryTopics {
		if err := c.connectCategory(category, topics); err != nil {
			log().Error().Err(err).Str("category", string(category)).Msg("Failed to connect to category")
			// Continue with other categories
		}
	}

	return nil
}

// connectCategory establishes a WebSocket connection for a specific category
func (c *BybitDataClient) connectCategory(category Category, topics []string) error {
	wsURL, ok := CategoryToWsURL[category]
	if !ok {
		return nil
	}

	log().Info().Str("url", wsURL).Str("category", string(category)).Int("topics", len(topics)).Msg("Connecting to Bybit WebSocket")

	// Create connection wrapper
	wsConn := &wsConnection{
		category: category,
		client:   c,
	}

	// Create event handler
	handler := &wsEventHandler{wsConn: wsConn}

	// Create client options
	option := &gws.ClientOption{
		Addr:             wsURL,
		ReadBufferSize:   wsReadBufferSize,
		CheckUtf8Enabled: false,
		NewDialer: func() (gws.Dialer, error) {
			return &ipv4Dialer{}, nil
		},
	}

	// Create WebSocket client
	conn, _, err := gws.NewClient(handler, option)
	if err != nil {
		return err
	}

	wsConn.connLock.Lock()
	wsConn.conn = conn
	wsConn.connLock.Unlock()
	wsConn.connected.Store(true)

	c.connsLock.Lock()
	c.conns[category] = wsConn
	c.connsLock.Unlock()

	// Start ReadLoop in a goroutine
	go func() {
		conn.ReadLoop()
		c.handleDisconnect(category)
	}()

	// Start ping loop
	go c.pingLoop(wsConn)

	// Subscribe to topics after connection
	c.sendSubscriptions(wsConn, topics)

	log().Info().Str("category", string(category)).Msg("Connected to Bybit WebSocket")
	return nil
}

// buildCategoryTopics groups subscriptions by category and builds topic lists
func (c *BybitDataClient) buildCategoryTopics() map[Category][]string {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()

	categoryTopics := make(map[Category][]string)

	for symbolID, opts := range c.depthSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
			continue
		}

		category, ok := ProductSlugToCategory[symbol.Product.Slug]
		if !ok {
			log().Error().Str("product", symbol.Product.Slug).Msg("Unsupported product type for Bybit")
			continue
		}

		// Topic format: orderbook.{depth}.{symbol}
		topic := "orderbook." + strconv.Itoa(int(opts.Depth)) + "." + symbol.Name
		categoryTopics[category] = append(categoryTopics[category], topic)
		c.registerTopic(topic, symbol)
	}

	for symbolID := range c.tradeSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for trade subscription")
			continue
		}

		category, ok := ProductSlugToCategory[symbol.Product.Slug]
		if !ok {
			log().Error().Str("product", symbol.Product.Slug).Msg("Unsupported product type for Bybit")
			continue
		}

		// Topic format: publicTrade.{symbol}
		topic := "publicTrade." + symbol.Name
		categoryTopics[category] = append(categoryTopics[category], topic)
		c.registerTopic(topic, symbol)
	}

	return categoryTopics
}

// registerTopic records the topic -> symbol mapping plus the symbolID ->
// tick multipliers the hot parse path needs.
func (c *BybitDataClient) registerTopic(topic string, symbol *cpanel.Symbol) {
	c.topicMapLock.Lock()
	c.topicToSymbol[topic] = symbol.ID
	c.precisions[symbol.ID] = newSymbolPrecision(symbol.PricePrecision, symbol.SizePrecision)
	c.topicMapLock.Unlock()
}

// sendSubscriptions sends subscription messages for topics
func (c *BybitDataClient) sendSubscriptions(wsConn *wsConnection, topics []string) {
	if len(topics) == 0 {
		return
	}

	// Build subscription message
	// {"op": "subscribe", "args": ["orderbook.50.BTCUSDT", ...]}
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"op":"subscribe","args":[`)
	for i, topic := range topics {
		if i > 0 {
			c.msgBuffer.WriteByte(',')
		}
		c.msgBuffer.WriteByte('"')
		c.msgBuffer.WriteString(topic)
		c.msgBuffer.WriteByte('"')
	}
	c.msgBuffer.WriteString(`]}`)

	wsConn.connLock.RLock()
	conn := wsConn.conn
	wsConn.connLock.RUnlock()

	if conn != nil {
		if err := conn.WriteMessage(gws.OpcodeText, c.msgBuffer.Bytes()); err != nil {
			log().Error().Err(err).Str("category", string(wsConn.category)).Msg("Failed to send subscription message")
		}
	}
}

// subscribeToDepthStream sends a subscription message for depth stream
func (c *BybitDataClient) subscribeToDepthStream(symbolID int, depth DepthLevel, category Category) {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
		return
	}

	topic := "orderbook." + strconv.Itoa(int(depth)) + "." + symbol.Name
	c.registerTopic(topic, symbol)

	c.connsLock.RLock()
	wsConn, exists := c.conns[category]
	c.connsLock.RUnlock()

	if exists {
		c.sendSubscriptions(wsConn, []string{topic})
	}
}

// subscribeToTradeStream sends a subscription message for trade stream
func (c *BybitDataClient) subscribeToTradeStream(symbolID int, category Category) {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for trade subscription")
		return
	}

	topic := "publicTrade." + symbol.Name
	c.registerTopic(topic, symbol)

	c.connsLock.RLock()
	wsConn, exists := c.conns[category]
	c.connsLock.RUnlock()

	if exists {
		c.sendSubscriptions(wsConn, []string{topic})
	}
}

// Disconnect closes all WebSocket connections
func (c *BybitDataClient) Disconnect() {
	c.shouldStop.Store(true)
	if c.cancel != nil {
		c.cancel()
	}

	c.connsLock.Lock()
	for category, wsConn := range c.conns {
		wsConn.connLock.Lock()
		if wsConn.conn != nil {
			_ = wsConn.conn.WriteClose(1000, nil)
			wsConn.conn = nil
		}
		wsConn.connLock.Unlock()
		wsConn.connected.Store(false)
		delete(c.conns, category)
	}
	c.connsLock.Unlock()

	log().Info().Msg("Disconnected from Bybit WebSocket")
}

// pingLoop sends periodic pings to keep the connection alive
func (c *BybitDataClient) pingLoop(wsConn *wsConnection) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			wsConn.connLock.RLock()
			conn := wsConn.conn
			wsConn.connLock.RUnlock()

			if conn != nil {
				// Bybit uses {"op": "ping"} format
				if err := conn.WriteMessage(gws.OpcodeText, []byte(`{"op":"ping"}`)); err != nil {
					log().Warn().Err(err).Str("category", string(wsConn.category)).Msg("Failed to send ping")
				}
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection for a category
func (c *BybitDataClient) handleDisconnect(category Category) {
	c.connsLock.RLock()
	wsConn, exists := c.conns[category]
	c.connsLock.RUnlock()

	if exists {
		wsConn.connected.Store(false)
	}

	if c.shouldStop.Load() {
		return
	}

	log().Warn().Str("category", string(category)).Msg("WebSocket disconnected, attempting to reconnect...")

	// Rebuild topics for this category
	categoryTopics := c.buildCategoryTopics()
	topics, ok := categoryTopics[category]
	if !ok || len(topics) == 0 {
		return
	}

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.connectCategory(category, topics); err != nil {
			log().Error().Err(err).Str("category", string(category)).Msg("Reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes a raw WebSocket message.
//
// P2-2 contract: all field extraction stays on []byte subslices of the
// connection read buffer — no string conversions, no per-message
// allocations. Topic resolution goes through a map[string]int lookup keyed
// with string(b) (compiler-optimized, non-allocating); only integer IDs
// cross into the arena. Nothing derived from data may be retained past this
// call (P2-1 buffer lifetime contract). ws carries the per-connection
// scratch buffers.
func (c *BybitDataClient) processMessage(ws *wsConnection, data []byte) {
	// Check if this is a pong or subscription response
	op, _, _, err := jsonparser.Get(data, "op")
	if err == nil {
		if bytes.Equal(op, bytesOpPong) || bytes.Equal(op, bytesOpSubscribe) {
			// Ignore pong and subscription responses
			return
		}
	}

	// Get topic to determine message type
	topic, _, _, err := jsonparser.Get(data, "topic")
	if err != nil {
		return
	}

	// Get message type (snapshot or delta)
	msgType, _, _, err := jsonparser.Get(data, "type")
	if err != nil {
		return
	}

	// Route to appropriate handler
	if bytes.HasPrefix(topic, bytesTopicOrderbook) {
		c.processOrderbook(ws, topic, msgType, data)
	} else if bytes.HasPrefix(topic, bytesTopicTrade) {
		c.processTrade(topic, data)
	}
}

// processOrderbook processes orderbook messages (snapshot or delta)
// Bybit orderbook format:
//
//	{
//	  "topic": "orderbook.50.BTCUSDT",
//	  "type": "snapshot" or "delta",
//	  "ts": 1672304484978,
//	  "data": {
//	    "s": "BTCUSDT",
//	    "b": [["price","size"],...],
//	    "a": [["price","size"],...],
//	    "u": 18521288,
//	    "seq": 7961638724
//	  },
//	  "cts": 1672304484976
//	}
func (c *BybitDataClient) processOrderbook(ws *wsConnection, topic, msgType []byte, data []byte) {
	c.topicMapLock.RLock()
	symbolID, ok := c.topicToSymbol[string(topic)]
	c.topicMapLock.RUnlock()

	if !ok {
		return // unknown topic (cold: only after a subscription bug)
	}

	// Get data object
	dataObj, _, _, err := jsonparser.Get(data, "data")
	if err != nil {
		log().Error().Err(err).Msg("Failed to get data field")
		return
	}

	// Parse timestamp
	ts, _ := jsonparser.GetInt(data, "ts")
	timestamp := uint64(ts) * 1_000_000 // Convert ms to ns

	// Parse update ID
	updateID, _ := jsonparser.GetInt(dataObj, "u")
	switch {
	case bytes.Equal(msgType, bytesTypeSnapshot):
		c.processDepthSnapshot(ws, symbolID, int(updateID), timestamp, dataObj)
	case bytes.Equal(msgType, bytesTypeDelta):
		c.processDepthUpdate(ws, symbolID, int(updateID), timestamp, dataObj)
	}
}

// getPrecision returns the cached tick multipliers for a symbol. On a miss
// (topic registered without going through registerTopic, e.g. in tests) it
// falls back to the catalog once and caches the result — the hot path
// afterwards is a read-locked map hit.
func (c *BybitDataClient) getPrecision(symbolID int) (symbolPrecision, bool) {
	c.topicMapLock.RLock()
	prec, ok := c.precisions[symbolID]
	c.topicMapLock.RUnlock()
	if ok {
		return prec, true
	}

	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth message")
		return symbolPrecision{}, false
	}
	prec = newSymbolPrecision(symbol.PricePrecision, symbol.SizePrecision)
	c.topicMapLock.Lock()
	c.precisions[symbolID] = prec
	c.topicMapLock.Unlock()
	return prec, true
}

// processDepthSnapshot handles snapshot messages
func (c *BybitDataClient) processDepthSnapshot(ws *wsConnection, symbolID, depthID int, timestamp uint64, dataObj []byte) {
	prec, ok := c.getPrecision(symbolID)
	if !ok {
		return
	}

	// Parse bids and asks into the per-connection scratch buffers. The
	// slices are only referenced until Encode below copies the levels into
	// the arena.
	ws.scratchBids = appendPriceLevels(ws.scratchBids[:0], dataObj, "b", prec)
	ws.scratchAsks = appendPriceLevels(ws.scratchAsks[:0], dataObj, "a", prec)

	// Create DepthSnapshot struct
	snapshot := event.DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      ws.scratchAsks,
		Bids:      ws.scratchBids,
	}

	// Calculate size and allocate buffer
	size := uint64(snapshot.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventDepthSnapshot, size)
	if !ok {
		return // dropped under overflow; orderbook re-syncs via DepthID gap
	}
	if err := snapshot.Encode(buf); err != nil {
		c.msgBus.Cancel(ref)
		return
	}
	c.msgBus.Publish(ref)
}

// processDepthUpdate handles delta messages
func (c *BybitDataClient) processDepthUpdate(ws *wsConnection, symbolID, depthID int, timestamp uint64, dataObj []byte) {
	prec, ok := c.getPrecision(symbolID)
	if !ok {
		return
	}

	// Parse bids and asks into the per-connection scratch buffers. The
	// slices are only referenced until Encode below copies the levels into
	// the arena.
	ws.scratchBids = appendPriceLevels(ws.scratchBids[:0], dataObj, "b", prec)
	ws.scratchAsks = appendPriceLevels(ws.scratchAsks[:0], dataObj, "a", prec)

	// Create DepthUpdate struct
	depthUpdate := event.DepthUpdate{
		SymbolID:        symbolID,
		PreviousDepthID: depthID - 1,
		DepthID:         depthID,
		CurrentDepthID:  depthID,
		NextDepthID:     depthID + 1,
		Timestamp:       timestamp,
		Asks:            ws.scratchAsks,
		Bids:            ws.scratchBids,
	}

	// Calculate size and allocate buffer
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

// processTrade processes trade messages
func (c *BybitDataClient) processTrade(topic []byte, data []byte) {
	c.topicMapLock.RLock()
	symbolID, ok := c.topicToSymbol[string(topic)]
	c.topicMapLock.RUnlock()

	if !ok {
		return
	}

	// Bybit trade format: data is an array of trades
	_, _ = jsonparser.ArrayEach(data, func(tradeData []byte, dataType jsonparser.ValueType, offset int, err error) {
		c.processTradeItem(symbolID, tradeData)
	}, "data")
}

// processTradeItem processes a single trade item
func (c *BybitDataClient) processTradeItem(symbolID int, tradeData []byte) {
	var tick event.Tick
	tick.SymbolID = symbolID

	// Parse timestamp
	ts, _ := jsonparser.GetInt(tradeData, "T")
	tick.Timestamp = uint64(ts) * 1_000_000 // Convert ms to ns

	// Parse price
	priceBytes, _, _, _ := jsonparser.Get(tradeData, "p")
	tick.Price = parseFloat64(priceBytes)

	// Parse quantity
	qtyBytes, _, _, _ := jsonparser.Get(tradeData, "v")
	tick.Qty = parseFloat64(qtyBytes)

	// Parse side: S = "Buy" or "Sell"
	side, _, _, _ := jsonparser.Get(tradeData, "S")
	if bytes.Equal(side, bytesSideSell) {
		tick.Side = common.SideSell
	} else {
		tick.Side = common.SideBuy
	}

	// Publish to event bus
	size := uint64(tick.GetBufferLength())
	ref, buf, ok := c.msgBus.Allocate(event.TopicEventTick, size)
	if !ok {
		return
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
	// Remove quotes if present
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
	wsConn *wsConnection
}

func (h *wsEventHandler) OnOpen(socket *gws.Conn) {
	log().Info().Str("category", string(h.wsConn.category)).Msg("Bybit WebSocket connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
}

func (h *wsEventHandler) OnClose(socket *gws.Conn, err error) {
	log().Info().Err(err).Str("category", string(h.wsConn.category)).Msg("Bybit WebSocket connection closed")
}

func (h *wsEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsEventHandler) OnPong(socket *gws.Conn, payload []byte) {
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
// is off), which is what makes the per-connection scratch buffers safe.
func (h *wsEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	h.wsConn.client.processMessage(h.wsConn, message.Bytes())
}
