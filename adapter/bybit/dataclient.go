package bybit

import (
	"bytes"
	"context"
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
	wsPingInterval      = 20 * time.Second // Bybit recommends 20s
	wsPingWait          = 10 * time.Second
)

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
	topicMapLock  sync.RWMutex

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
	}
}

// HasSub returns true if there are any subscriptions configured
func (c *BybitDataClient) HasSub() bool {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()
	return len(c.depthSubs) > 0 || len(c.tradeSubs) > 0
}

// SubscribeDepthUpdate subscribes to depth update stream for a symbol
// If options is nil, defaults to DepthLevel50
func (c *BybitDataClient) SubscribeDepthUpdate(symbolID int, options *DepthSubscriptionOptions) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	opts := &DepthSubscriptionOptions{Depth: DepthLevel50}
	if options != nil {
		opts.Depth = options.Depth
	}
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

// SubscribeTrade subscribes to trade stream for a symbol
func (c *BybitDataClient) SubscribeTrade(symbolID int) {
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

		c.topicMapLock.Lock()
		c.topicToSymbol[topic] = symbolID
		c.topicMapLock.Unlock()
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

		c.topicMapLock.Lock()
		c.topicToSymbol[topic] = symbolID
		c.topicMapLock.Unlock()
	}

	return categoryTopics
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

	c.topicMapLock.Lock()
	c.topicToSymbol[topic] = symbolID
	c.topicMapLock.Unlock()

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

	c.topicMapLock.Lock()
	c.topicToSymbol[topic] = symbolID
	c.topicMapLock.Unlock()

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

// processMessage processes a raw WebSocket message
func (c *BybitDataClient) processMessage(data []byte) {
	// Check if this is a pong or subscription response
	op, err := jsonparser.GetString(data, "op")
	if err == nil {
		if op == "pong" || op == "subscribe" {
			// Ignore pong and subscription responses
			return
		}
	}

	// Get topic to determine message type
	topic, err := jsonparser.GetString(data, "topic")
	if err != nil {
		return
	}

	// Get message type (snapshot or delta)
	msgType, err := jsonparser.GetString(data, "type")
	if err != nil {
		return
	}

	// Route to appropriate handler
	if strings.HasPrefix(topic, "orderbook.") {
		c.processOrderbook(topic, msgType, data)
	} else if strings.HasPrefix(topic, "publicTrade.") {
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
func (c *BybitDataClient) processOrderbook(topic, msgType string, data []byte) {
	c.topicMapLock.RLock()
	symbolID, ok := c.topicToSymbol[topic]
	c.topicMapLock.RUnlock()

	if !ok {
		log().Warn().Str("topic", topic).Msg("Received message for unknown topic")
		return
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
	switch msgType {
	case "snapshot":
		c.processDepthSnapshot(symbolID, int(updateID), timestamp, dataObj)
	case "delta":
		c.processDepthUpdate(symbolID, int(updateID), timestamp, dataObj)
	}
}

// processDepthSnapshot handles snapshot messages
func (c *BybitDataClient) processDepthSnapshot(symbolID, depthID int, timestamp uint64, dataObj []byte) {
	// Get symbol precision for tick computation
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth snapshot")
		return
	}

	// Parse bids and asks with precision for PriceTick/QuantityTick
	bids := c.parsePriceLevels(dataObj, "b", symbol.PricePrecision, symbol.SizePrecision)
	asks := c.parsePriceLevels(dataObj, "a", symbol.PricePrecision, symbol.SizePrecision)

	// Create DepthSnapshot struct
	snapshot := event.DepthSnapshot{
		SymbolID:  symbolID,
		DepthID:   depthID,
		Timestamp: timestamp,
		Asks:      asks,
		Bids:      bids,
	}

	// Calculate size and allocate buffer
	size := uint64(snapshot.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)

	// Encode and publish
	snapshot.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventDepthSnapshot,
		Index:  offset,
		Length: size,
	})
}

// processDepthUpdate handles delta messages
func (c *BybitDataClient) processDepthUpdate(symbolID, depthID int, timestamp uint64, dataObj []byte) {
	// Get symbol precision for tick computation
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth update")
		return
	}

	// Parse bids and asks with precision for PriceTick/QuantityTick
	bids := c.parsePriceLevels(dataObj, "b", symbol.PricePrecision, symbol.SizePrecision)
	asks := c.parsePriceLevels(dataObj, "a", symbol.PricePrecision, symbol.SizePrecision)

	// Create DepthUpdate struct
	depthUpdate := event.DepthUpdate{
		SymbolID:        symbolID,
		PreviousDepthID: depthID - 1,
		DepthID:         depthID,
		CurrentDepthID:  depthID,
		NextDepthID:     depthID + 1,
		Timestamp:       timestamp,
		Asks:            asks,
		Bids:            bids,
	}

	// Calculate size and allocate buffer
	size := uint64(depthUpdate.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)

	// Encode and publish
	depthUpdate.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventDepthUpdate,
		Index:  offset,
		Length: size,
	})
}

// processTrade processes trade messages
func (c *BybitDataClient) processTrade(topic string, data []byte) {
	c.topicMapLock.RLock()
	symbolID, ok := c.topicToSymbol[topic]
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
	sideStr, _ := jsonparser.GetString(tradeData, "S")
	if sideStr == "Sell" {
		tick.Side = 1 // SideSell
	} else {
		tick.Side = 0 // SideBuy
	}

	// Publish to event bus
	size := uint64(tick.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	tick.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})
}

// parsePriceLevels parses an array of [price, qty] arrays from JSON.
// Uses pricePrecision and sizePrecision to compute PriceTick and QuantityTick.
func (c *BybitDataClient) parsePriceLevels(data []byte, key string, pricePrecision, sizePrecision int) []common.PriceLevel {
	// First pass: count elements
	count := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		count++
	}, key)

	if count == 0 {
		return nil
	}

	// Allocate slice
	levels := make([]common.PriceLevel, count)

	// Second pass: parse values
	idx := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if idx >= count {
			return
		}

		// Parse [price, qty] array
		elemIdx := 0
		_, _ = jsonparser.ArrayEach(value, func(elem []byte, dt jsonparser.ValueType, off int, e error) {
			// Remove quotes if present
			if len(elem) >= 2 && elem[0] == '"' {
				elem = elem[1 : len(elem)-1]
			}
			if elemIdx == 0 {
				levels[idx].Price = parseFloat64(elem)
			} else if elemIdx == 1 {
				levels[idx].Quantity = parseFloat64(elem)
			}
			elemIdx++
		})
		levels[idx].PriceTick = common.PriceToTick(levels[idx].Price, pricePrecision)
		levels[idx].QuantityTick = common.QuantityToTick(levels[idx].Quantity, sizePrecision)
		idx++
	}, key)

	return levels[:idx]
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

func (h *wsEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	h.wsConn.client.processMessage(message.Bytes())
}
