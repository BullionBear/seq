package binance

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
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/buger/jsonparser"
	"github.com/lxzan/gws"
)

var log = logger.Get()

const (
	// WebSocket connection settings
	wsReadBufferSize    = 64 * 1024 // 64KB read buffer
	wsReconnectInterval = 5 * time.Second
	wsPingInterval      = 30 * time.Second
	wsPingWait          = 10 * time.Second

	// Binance stream names
	streamDepthUpdate = "depth@100ms"
	streamTrade       = "trade"
)

// BinanceSpotDataClient handles Binance spot market data via WebSocket
type BinanceSpotDataClient struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus

	// WebSocket connection
	conn     *gws.Conn
	connLock sync.RWMutex

	// Subscription management
	depthSubs map[int]*DepthSubscriptionOptions // symbolID -> options
	tradeSubs map[int]struct{}                  // symbolID -> exists
	subsLock  sync.RWMutex

	// Stream to symbolID mapping for fast lookup during message processing
	streamToSymbol map[string]int // "btcusdt@depth@100ms" -> symbolID
	streamMapLock  sync.RWMutex

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
func NewBinanceSpotDataClient(catalog *catalog.Catalog, eventBus *evbus.EventBus) *BinanceSpotDataClient {
	return &BinanceSpotDataClient{
		catalog:        catalog,
		eventBus:       eventBus,
		depthSubs:      make(map[int]*DepthSubscriptionOptions, 64),
		tradeSubs:      make(map[int]struct{}, 64),
		streamToSymbol: make(map[string]int, 128),
	}
}

// SubscribeDepthUpdate subscribes to depth update stream for a symbol
func (c *BinanceSpotDataClient) SubscribeDepthUpdate(symbolID int, options *DepthSubscriptionOptions) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	opts := &DepthSubscriptionOptions{SymbolID: symbolID}
	if options != nil {
		opts.Limit = options.Limit
	}
	c.depthSubs[symbolID] = opts

	// If already connected, send subscribe message
	if c.connected.Load() {
		c.subscribeToStream(symbolID, streamDepthUpdate)
	}
}

// SubscribeTrade subscribes to trade stream for a symbol
func (c *BinanceSpotDataClient) SubscribeTrade(symbolID int) {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	c.tradeSubs[symbolID] = struct{}{}

	// If already connected, send subscribe message
	if c.connected.Load() {
		c.subscribeToStream(symbolID, streamTrade)
	}
}

// Connect establishes the WebSocket connection and starts processing
func (c *BinanceSpotDataClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	// Build combined stream URL
	streams := c.buildStreamList()
	if len(streams) == 0 {
		log.Warn().Msg("No subscriptions configured, skipping Binance connection")
		return nil
	}

	url := c.buildStreamURL(streams)
	log.Info().Str("url", url).Int("streams", len(streams)).Msg("Connecting to Binance WebSocket")

	// Create event handler
	handler := &wsEventHandler{client: c}

	// Create client options with optimized settings
	option := &gws.ClientOption{
		Addr:             url,
		ReadBufferSize:   wsReadBufferSize,
		CheckUtf8Enabled: false, // Disable UTF-8 check for performance
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

	log.Info().Msg("Connected to Binance WebSocket")
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

	log.Info().Msg("Disconnected from Binance WebSocket")
}

// buildStreamList builds the list of streams to subscribe
func (c *BinanceSpotDataClient) buildStreamList() []string {
	c.subsLock.RLock()
	defer c.subsLock.RUnlock()

	streams := make([]string, 0, len(c.depthSubs)+len(c.tradeSubs))

	for symbolID := range c.depthSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log.Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for depth subscription")
			continue
		}
		streamName := strings.ToLower(symbol.Name) + "@" + streamDepthUpdate
		streams = append(streams, streamName)

		c.streamMapLock.Lock()
		c.streamToSymbol[streamName] = symbolID
		c.streamMapLock.Unlock()
	}

	for symbolID := range c.tradeSubs {
		symbol, err := c.catalog.GetSymbol(symbolID)
		if err != nil {
			log.Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for trade subscription")
			continue
		}
		streamName := strings.ToLower(symbol.Name) + "@" + streamTrade
		streams = append(streams, streamName)

		c.streamMapLock.Lock()
		c.streamToSymbol[streamName] = symbolID
		c.streamMapLock.Unlock()
	}

	return streams
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
		log.Error().Err(err).Int("symbolID", symbolID).Msg("Failed to get symbol for subscription")
		return
	}

	streamName := strings.ToLower(symbol.Name) + "@" + streamType

	c.streamMapLock.Lock()
	c.streamToSymbol[streamName] = symbolID
	c.streamMapLock.Unlock()

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
			log.Error().Err(err).Str("stream", streamName).Msg("Failed to send subscription message")
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
					log.Warn().Err(err).Msg("Failed to send ping")
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

	log.Warn().Msg("WebSocket disconnected, attempting to reconnect...")

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.Connect(c.ctx); err != nil {
			log.Error().Err(err).Msg("Reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes a raw WebSocket message using jsonparser
func (c *BinanceSpotDataClient) processMessage(data []byte) {
	// Check if this is a combined stream message (has "stream" field)
	stream, err := jsonparser.GetString(data, "stream")
	if err == nil {
		// Combined stream format: {"stream":"...","data":{...}}
		msgData, _, _, err := jsonparser.Get(data, "data")
		if err != nil {
			log.Error().Err(err).Msg("Failed to get data field from combined stream message")
			return
		}
		c.processStreamMessage(stream, msgData)
		return
	}

	// Single stream format - determine type from event field
	eventType, err := jsonparser.GetString(data, "e")
	if err != nil {
		// Might be a subscription response or error
		return
	}

	// For single stream, we need to determine the stream name from the symbol
	symbol, _ := jsonparser.GetString(data, "s")
	if symbol == "" {
		return
	}

	switch eventType {
	case "depthUpdate":
		streamName := strings.ToLower(symbol) + "@" + streamDepthUpdate
		c.processStreamMessage(streamName, data)
	case "trade":
		streamName := strings.ToLower(symbol) + "@" + streamTrade
		c.processStreamMessage(streamName, data)
	}
}

// processStreamMessage routes a message to the appropriate handler
func (c *BinanceSpotDataClient) processStreamMessage(stream string, data []byte) {
	c.streamMapLock.RLock()
	symbolID, ok := c.streamToSymbol[stream]
	c.streamMapLock.RUnlock()

	if !ok {
		log.Warn().Str("stream", stream).Msg("Received message for unknown stream")
		return
	}

	// Determine message type from stream name
	if strings.Contains(stream, "depth") {
		c.processDepthUpdate(symbolID, data)
	} else if strings.Contains(stream, "trade") {
		c.processTrade(symbolID, data)
	}
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

	// Parse bids
	depthUpdate.Bids = c.parsePriceLevels(data, "b")

	// Parse asks
	depthUpdate.Asks = c.parsePriceLevels(data, "a")

	// Publish to event bus
	c.eventBus.PublishDepthUpdate(depthUpdate)
}

// processTrade parses and publishes a trade event
// Binance trade format:
//
//	{
//	  "e": "trade",
//	  "E": 123456789,   // Event time
//	  "s": "BTCUSDT",   // Symbol
//	  "t": 12345,       // Trade ID
//	  "p": "0.001",     // Price
//	  "q": "100",       // Quantity
//	  "T": 123456785,   // Trade time
//	  "m": true,        // Is buyer maker
//	  "M": true         // Ignore
//	}
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
	c.eventBus.PublishTick(tick)
}

// parsePriceLevels parses an array of [price, qty] arrays from JSON
func (c *BinanceSpotDataClient) parsePriceLevels(data []byte, key string) []event.PriceLevel {
	// First pass: count elements
	count := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		count++
	}, key)

	if count == 0 {
		return nil
	}

	// Allocate slice for price levels
	levels := make([]event.PriceLevel, count)

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
		idx++
	}, key)

	return levels[:idx]
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
	log.Info().Msg("Binance WebSocket connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
}

func (h *wsEventHandler) OnClose(socket *gws.Conn, err error) {
	log.Info().Err(err).Msg("Binance WebSocket connection closed")
}

func (h *wsEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsEventHandler) OnPong(socket *gws.Conn, payload []byte) {
	// Pong received, connection is alive
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
}

func (h *wsEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	// Reset deadline on message received
	_ = socket.SetDeadline(time.Now().Add(wsPingInterval + wsPingWait))
	// Process the message
	h.client.processMessage(message.Bytes())
}
