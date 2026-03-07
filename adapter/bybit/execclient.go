package bybit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/buger/jsonparser"
	"github.com/lxzan/gws"
)

const (
	// WebSocket connection settings for execution
	wsExecReadBufferSize = 64 * 1024 // 64KB read buffer
	wsExecPingInterval   = 20 * time.Second
	wsExecPingWait       = 10 * time.Second
	wsExecRecvWindow     = 5000 // 5 seconds
	wsExecAuthExpiry     = 1    // 1 second for auth signature
)

// BybitPrivateStreamClient handles Bybit private stream WebSocket connection
// Endpoint: wss://stream.bybit.com/v5/private
// Purpose: Listen to private events (order updates, executions, wallet updates)
// Topics: order, execution, wallet, position
type BybitPrivateStreamClient struct {
	catalog   *catalog.Catalog
	msgBus    *msgbus.MsgBus
	accountID int
	account   cpanel.Account
	apiKey    cpanel.APIKey

	// WebSocket connection
	conn     *gws.Conn
	connLock sync.RWMutex

	// Connection state
	connected     atomic.Bool
	authenticated atomic.Bool
	shouldStop    atomic.Bool

	// Pending subscriptions (stored before Connect, sent after Connect)
	pendingTopics map[string]bool
	topicsMu      sync.Mutex

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building messages
	msgBuffer bytes.Buffer
	bufLock   sync.Mutex
}

// NewBybitPrivateStreamClient creates a new Bybit private stream client
func NewBybitPrivateStreamClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus, accountID int, apiKeyName string) (*BybitPrivateStreamClient, error) {
	account, err := catalog.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	apiKey, err := account.GetAPI(apiKeyName)
	if err != nil {
		return nil, err
	}

	return &BybitPrivateStreamClient{
		catalog:       catalog,
		msgBus:        msgBus,
		accountID:     accountID,
		account:       *account,
		apiKey:        *apiKey,
		pendingTopics: make(map[string]bool),
	}, nil
}

// Connect establishes the WebSocket connection for private stream
func (c *BybitPrivateStreamClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	handler := &wsPrivateStreamHandler{client: c}

	option := &gws.ClientOption{
		Addr:             WsPrivateURL,
		ReadBufferSize:   wsExecReadBufferSize,
		CheckUtf8Enabled: false,
		NewDialer: func() (gws.Dialer, error) {
			return &ipv4Dialer{}, nil
		},
	}

	conn, _, err := gws.NewClient(handler, option)
	if err != nil {
		return err
	}

	c.connLock.Lock()
	c.conn = conn
	c.connLock.Unlock()
	c.connected.Store(true)

	// Start ReadLoop in a goroutine
	go func() {
		conn.ReadLoop()
		c.handleDisconnect()
	}()

	// Start ping loop
	go c.pingLoop()

	// Authenticate immediately after connection
	if err := c.authenticate(); err != nil {
		log().Error().Err(err).Msg("Failed to authenticate private stream")
		return err
	}

	// Subscribe to all pending topics after authentication
	if err := c.subscribePendingTopics(); err != nil {
		log().Error().Err(err).Msg("Failed to subscribe to pending topics")
		return err
	}

	log().Info().Int("accountID", c.accountID).Msg("Connected to Bybit Private Stream WebSocket")
	return nil
}

// Disconnect closes the WebSocket connection
func (c *BybitPrivateStreamClient) Disconnect() {
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
	c.authenticated.Store(false)

	log().Info().Int("accountID", c.accountID).Msg("Disconnected from Bybit Private Stream WebSocket")
}

// Subscribe registers topics to subscribe to.
// If already connected, sends the subscription immediately.
// If not connected, stores the topics and subscribes on Connect().
// This method is idempotent - subscribing to the same topic multiple times has no effect.
// Topics: order, execution, wallet, position
func (c *BybitPrivateStreamClient) Subscribe(topics []string) error {
	if len(topics) == 0 {
		return nil
	}

	// Add topics to pending set (idempotent)
	c.topicsMu.Lock()
	newTopics := make([]string, 0, len(topics))
	for _, topic := range topics {
		if !c.pendingTopics[topic] {
			c.pendingTopics[topic] = true
			newTopics = append(newTopics, topic)
		}
	}
	c.topicsMu.Unlock()

	// If not connected yet, topics will be subscribed on Connect()
	if !c.connected.Load() || !c.authenticated.Load() {
		return nil
	}

	// If already connected and there are new topics, subscribe now
	if len(newTopics) > 0 {
		return c.sendSubscription(newTopics)
	}

	return nil
}

// sendSubscription sends a subscription message for the given topics
func (c *BybitPrivateStreamClient) sendSubscription(topics []string) error {
	if len(topics) == 0 {
		return nil
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build subscription message
	// {"op": "subscribe", "args": ["order", "execution", "wallet"]}
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

	return c.sendMessage(c.msgBuffer.Bytes())
}

// subscribePendingTopics subscribes to all topics that were registered before Connect()
func (c *BybitPrivateStreamClient) subscribePendingTopics() error {
	c.topicsMu.Lock()
	topics := make([]string, 0, len(c.pendingTopics))
	for topic := range c.pendingTopics {
		topics = append(topics, topic)
	}
	c.topicsMu.Unlock()

	if len(topics) == 0 {
		return nil
	}

	log().Debug().Strs("topics", topics).Int("accountID", c.accountID).Msg("Subscribing to pending topics")
	return c.sendSubscription(topics)
}

// SubscribeOrderUpdate subscribes to order updates
func (c *BybitPrivateStreamClient) SubscribeOrderUpdate() error {
	return c.Subscribe([]string{"order"})
}

// SubscribeExecution subscribes to execution (fill) updates
func (c *BybitPrivateStreamClient) SubscribeExecution() error {
	return c.Subscribe([]string{"execution"})
}

// SubscribeWallet subscribes to wallet (balance) updates
func (c *BybitPrivateStreamClient) SubscribeWallet() error {
	return c.Subscribe([]string{"wallet"})
}

// authenticate sends authentication message to Bybit
// Auth format: {"op": "auth", "args": [apiKey, expires, signature]}
func (c *BybitPrivateStreamClient) authenticate() error {
	expires := time.Now().UnixMilli() + wsExecAuthExpiry*1000

	// Build signature payload: "GET/realtime" + expires
	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	c.msgBuffer.Reset()
	c.msgBuffer.WriteString("GET/realtime")
	c.msgBuffer.WriteString(strconv.FormatInt(expires, 10))

	signature := c.signHMAC(c.msgBuffer.Bytes())

	// Build auth message
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"op":"auth","args":["`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`",`)
	c.msgBuffer.WriteString(strconv.FormatInt(expires, 10))
	c.msgBuffer.WriteString(`,"`)
	c.msgBuffer.WriteString(signature)
	c.msgBuffer.WriteString(`"]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// signHMAC signs the payload with HMAC-SHA256
func (c *BybitPrivateStreamClient) signHMAC(payload []byte) string {
	h := hmac.New(sha256.New, []byte(c.apiKey.Secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// sendMessage sends a WebSocket message
func (c *BybitPrivateStreamClient) sendMessage(msg []byte) error {
	c.connLock.RLock()
	conn := c.conn
	c.connLock.RUnlock()

	if conn == nil {
		return &HTTPError{StatusCode: 0, Body: "WebSocket not connected"}
	}

	return conn.WriteMessage(gws.OpcodeText, msg)
}

// pingLoop sends periodic pings to keep the connection alive
func (c *BybitPrivateStreamClient) pingLoop() {
	ticker := time.NewTicker(wsExecPingInterval)
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
				if err := conn.WriteMessage(gws.OpcodeText, []byte(`{"op":"ping"}`)); err != nil {
					log().Warn().Err(err).Msg("Failed to send ping to private stream")
				}
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *BybitPrivateStreamClient) handleDisconnect() {
	c.connected.Store(false)
	c.authenticated.Store(false)

	if c.shouldStop.Load() {
		return
	}

	log().Warn().Int("accountID", c.accountID).Msg("Private stream disconnected, attempting to reconnect...")

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.Connect(c.ctx); err != nil {
			log().Error().Err(err).Msg("Private stream reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes incoming WebSocket messages
func (c *BybitPrivateStreamClient) processMessage(data []byte) {
	// Check for op response (pong, auth, subscribe)
	op, err := jsonparser.GetString(data, "op")
	if err == nil {
		c.processOpResponse(op, data)
		return
	}

	// Check for private stream data (has "topic" field)
	topic, err := jsonparser.GetString(data, "topic")
	if err != nil {
		return
	}

	// Route to appropriate handler based on topic
	switch topic {
	case "order":
		c.processOrderTopic(data)
	case "execution":
		c.processExecutionTopic(data)
	case "wallet":
		c.processWalletTopic(data)
	default:
		log().Debug().Str("topic", topic).Msg("Received unhandled private stream topic")
	}
}

// processOpResponse handles operation responses (auth, subscribe, pong)
func (c *BybitPrivateStreamClient) processOpResponse(op string, data []byte) {
	switch op {
	case "pong":
		// Pong response - connection alive
		return
	case "auth":
		success, _ := jsonparser.GetBoolean(data, "success")
		if success {
			c.authenticated.Store(true)
			log().Info().Int("accountID", c.accountID).Msg("Private stream authenticated")
		} else {
			retMsg, _ := jsonparser.GetString(data, "ret_msg")
			log().Error().Str("msg", retMsg).Msg("Private stream authentication failed")
		}
	case "subscribe":
		success, _ := jsonparser.GetBoolean(data, "success")
		if !success {
			retMsg, _ := jsonparser.GetString(data, "ret_msg")
			log().Error().Str("msg", retMsg).Msg("Private stream subscription failed")
		}
	}
}

// processOrderTopic processes order update messages
// Bybit order format:
//
//	{
//	  "topic": "order",
//	  "data": [{
//	    "symbol": "BTCUSDT",
//	    "orderId": "xxx",
//	    "orderLinkId": "xxx",
//	    "side": "Buy",
//	    "orderType": "Limit",
//	    "orderStatus": "New",
//	    "qty": "0.001",
//	    "price": "50000",
//	    "cumExecQty": "0",
//	    "cumExecValue": "0",
//	    "createdTime": "1672364262462",
//	    "updatedTime": "1672364262462"
//	  }]
//	}
func (c *BybitPrivateStreamClient) processOrderTopic(data []byte) {
	_, _ = jsonparser.ArrayEach(data, func(orderData []byte, _ jsonparser.ValueType, _ int, _ error) {
		c.processOrderItem(orderData)
	}, "data")
}

// processOrderItem processes a single order update
func (c *BybitPrivateStreamClient) processOrderItem(data []byte) {
	orderIDStr, _ := jsonparser.GetString(data, "orderId")
	orderLinkIDStr, _ := jsonparser.GetString(data, "orderLinkId")
	status, _ := jsonparser.GetString(data, "orderStatus")
	cumExecQtyStr, _ := jsonparser.GetString(data, "cumExecQty")
	updatedTimeStr, _ := jsonparser.GetString(data, "updatedTime")
	createdTimeStr, _ := jsonparser.GetString(data, "createdTime")

	// Parse IDs - orderLinkId is our clientOrderID
	clientOrderID, _ := strconv.Atoi(orderLinkIDStr)
	orderID, _ := strconv.Atoi(orderIDStr)
	cumExecQty := parseFloat64([]byte(cumExecQtyStr))
	updatedTime, _ := strconv.ParseInt(updatedTimeStr, 10, 64)
	createdTime, _ := strconv.ParseInt(createdTimeStr, 10, 64)

	updatedAt := uint64(updatedTime) * 1_000_000 // Convert ms to ns
	createdAt := uint64(createdTime) * 1_000_000

	// Publish appropriate order event based on status
	// Bybit order statuses: New, PartiallyFilled, Filled, Cancelled, Rejected, PartiallyFilledCanceled
	switch status {
	case "New":
		c.publishOrderAccepted(clientOrderID, orderID, createdAt)

	case "PartiallyFilled":
		c.publishOrderPartiallyFilled(clientOrderID, orderID, cumExecQty, updatedAt)

	case "Filled":
		c.publishOrderFilled(clientOrderID, orderID, cumExecQty, updatedAt)

	case "Cancelled", "PartiallyFilledCanceled":
		c.publishOrderCanceled(clientOrderID, orderID, updatedAt)

	case "Rejected":
		rejectReason, _ := jsonparser.GetString(data, "rejectReason")
		c.publishOrderRejected(clientOrderID, orderID, rejectReason, updatedAt)

	default:
		c.publishOrderUnknownStatus(clientOrderID, orderID, status)
	}

	log().Debug().
		Int("accountID", c.accountID).
		Int("orderID", orderID).
		Str("status", status).
		Msg("Processed order update")
}

// processExecutionTopic processes execution (fill) messages
// Bybit execution format:
//
//	{
//	  "topic": "execution",
//	  "data": [{
//	    "symbol": "BTCUSDT",
//	    "orderId": "xxx",
//	    "orderLinkId": "xxx",
//	    "execId": "xxx",
//	    "execPrice": "50000",
//	    "execQty": "0.001",
//	    "execFee": "0.00001",
//	    "feeCurrency": "USDT",
//	    "execTime": "1672364262462"
//	  }]
//	}
func (c *BybitPrivateStreamClient) processExecutionTopic(data []byte) {
	_, _ = jsonparser.ArrayEach(data, func(execData []byte, _ jsonparser.ValueType, _ int, _ error) {
		c.processExecutionItem(execData)
	}, "data")
}

// processExecutionItem processes a single execution (fill)
func (c *BybitPrivateStreamClient) processExecutionItem(data []byte) {
	orderIDStr, _ := jsonparser.GetString(data, "orderId")
	orderLinkIDStr, _ := jsonparser.GetString(data, "orderLinkId")
	execIDStr, _ := jsonparser.GetString(data, "execId")
	execPriceStr, _ := jsonparser.GetString(data, "execPrice")
	execQtyStr, _ := jsonparser.GetString(data, "execQty")
	execFeeStr, _ := jsonparser.GetString(data, "execFee")
	feeCurrency, _ := jsonparser.GetString(data, "feeCurrency")
	execTimeStr, _ := jsonparser.GetString(data, "execTime")
	symbolStr, _ := jsonparser.GetString(data, "symbol")
	sideStr, _ := jsonparser.GetString(data, "side")

	clientOrderID, _ := strconv.Atoi(orderLinkIDStr)
	orderID, _ := strconv.Atoi(orderIDStr)
	fillID, _ := strconv.Atoi(execIDStr)
	execPrice := parseFloat64([]byte(execPriceStr))
	execQty := parseFloat64([]byte(execQtyStr))
	execFee := parseFloat64([]byte(execFeeStr))
	execTime, _ := strconv.ParseInt(execTimeStr, 10, 64)
	symbolID := 0
	if c.catalog != nil {
		if sym, err := c.catalog.GetSymbolByExchangeAndName(c.account.Exchange, symbolStr); err == nil {
			symbolID = sym.ID
		}
	}
	side := parseSide(sideStr)
	isMaker, _ := jsonparser.GetBoolean(data, "isMaker")

	fill := event.Execution{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		SymbolID:      symbolID,
		Side:          side,
		IsMaker:       isMaker,
		FillID:        fillID,
		FilledQty:     execQty,
		FilledPrice:   execPrice,
		FeeCcyID:      c.getTokenIDByName(feeCurrency),
		FeeQty:        execFee,
		FilledAt:      uint64(execTime) * 1_000_000,
	}

	size := uint64(fill.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	fill.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventExecution,
		Index:  offset,
		Length: size,
	})

	log().Debug().
		Int("accountID", c.accountID).
		Int("orderID", orderID).
		Float64("execQty", execQty).
		Float64("execPrice", execPrice).
		Msg("Published fill from execution topic")
}

// processWalletTopic processes wallet (balance) update messages
// Bybit wallet format:
//
//	{
//	  "topic": "wallet",
//	  "data": [{
//	    "accountType": "UNIFIED",
//	    "coin": [{
//	      "coin": "USDT",
//	      "walletBalance": "10000",
//	      "availableToWithdraw": "9000",
//	      "locked": "1000"
//	    }]
//	  }]
//	}
func (c *BybitPrivateStreamClient) processWalletTopic(data []byte) {
	_, _ = jsonparser.ArrayEach(data, func(walletData []byte, _ jsonparser.ValueType, _ int, _ error) {
		c.processWalletItem(walletData)
	}, "data")
}

// processWalletItem processes a single wallet update
func (c *BybitPrivateStreamClient) processWalletItem(data []byte) {
	// Count coins first
	coinCount := 0
	_, _ = jsonparser.ArrayEach(data, func(_ []byte, _ jsonparser.ValueType, _ int, _ error) {
		coinCount++
	}, "coin")

	if coinCount == 0 {
		return
	}

	balances := make([]common.Balance, 0, coinCount)

	_, _ = jsonparser.ArrayEach(data, func(coinData []byte, _ jsonparser.ValueType, _ int, _ error) {
		coin, _ := jsonparser.GetString(coinData, "coin")
		walletBalanceStr, _ := jsonparser.GetString(coinData, "walletBalance")
		availableStr, _ := jsonparser.GetString(coinData, "availableToWithdraw")
		lockedStr, _ := jsonparser.GetString(coinData, "locked")

		total := parseFloat64([]byte(walletBalanceStr))
		available := parseFloat64([]byte(availableStr))
		locked := parseFloat64([]byte(lockedStr))

		if total > 0 || available > 0 || locked > 0 {
			balances = append(balances, common.Balance{
				TokenID:   c.getTokenIDByName(coin),
				Available: available,
				Locked:    locked,
				Total:     total,
			})
		}
	}, "coin")

	if len(balances) == 0 {
		return
	}

	balanceUpdate := event.BalanceUpdate{
		AccountID: c.accountID,
		Balances:  balances,
		UpdatedAt: uint64(time.Now().UnixNano()),
	}

	size := uint64(balanceUpdate.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	balanceUpdate.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventBalanceUpdate,
		Index:  offset,
		Length: size,
	})

	log().Debug().
		Int("accountID", c.accountID).
		Int("balanceCount", len(balances)).
		Msg("Published balance update from wallet topic")
}

// publishOrderAccepted publishes an OrderAccepted event
func (c *BybitPrivateStreamClient) publishOrderAccepted(clientOrderID, orderID int, createdAt uint64) {
	e := event.OrderAccepted{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		CreatedAt:     createdAt,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderAccepted,
		Index:  offset,
		Length: size,
	})
}

// publishOrderPartiallyFilled publishes an OrderPartiallyFilled event
func (c *BybitPrivateStreamClient) publishOrderPartiallyFilled(clientOrderID, orderID int, executedQty float64, updatedAt uint64) {
	e := event.OrderPartiallyFilled{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		ExecutedQty:   executedQty,
		UpdatedAt:     updatedAt,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderPartialFill,
		Index:  offset,
		Length: size,
	})
}

// publishOrderFilled publishes an OrderFilled event
func (c *BybitPrivateStreamClient) publishOrderFilled(clientOrderID, orderID int, executedQty float64, updatedAt uint64) {
	e := event.OrderFilled{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		ExecutedQty:   executedQty,
		UpdatedAt:     updatedAt,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderFilled,
		Index:  offset,
		Length: size,
	})
}

// publishOrderCanceled publishes an OrderCanceled event
func (c *BybitPrivateStreamClient) publishOrderCanceled(clientOrderID, orderID int, updatedAt uint64) {
	e := event.OrderCanceled{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		UpdatedAt:     updatedAt,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderCanceled,
		Index:  offset,
		Length: size,
	})
}

// publishOrderRejected publishes an OrderRejected event.
// updatedAt is the exchange timestamp when available; otherwise use received time (e.g. time.Now().UnixNano()).
func (c *BybitPrivateStreamClient) publishOrderRejected(clientOrderID, orderID int, reason string, updatedAt uint64) {
	e := event.OrderRejected{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		ErrorCode:     0,
		UpdatedAt:     updatedAt,
		Msg:           reason,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderRejected,
		Index:  offset,
		Length: size,
	})
}

// publishOrderUnknownStatus publishes an OrderUnknownStatus event
func (c *BybitPrivateStreamClient) publishOrderUnknownStatus(clientOrderID, orderID int, status string) {
	e := event.OrderUnknownStatus{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		Msg:           status,
	}
	size := uint64(e.GetBufferLength())
	offset, buf := c.msgBus.Allocate(size)
	e.Encode(buf)
	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderUnknownStatus,
		Index:  offset,
		Length: size,
	})
}

// getTokenID gets the token ID from the catalog by asset name
func (c *BybitPrivateStreamClient) getTokenIDByName(name string) int {
	if c.catalog != nil {
		if tokenID, err := c.catalog.GetTokenIDByName(name); err == nil {
			return tokenID
		}
	}
	return 0
}

// parseSide maps Bybit side string ("Buy"/"Sell") to common.Side.
func parseSide(s string) common.Side {
	switch strings.ToUpper(s) {
	case "BUY":
		return common.SideBuy
	case "SELL":
		return common.SideSell
	default:
		return common.SideUnknown
	}
}

// wsPrivateStreamHandler implements gws.Event interface for private stream WebSocket
type wsPrivateStreamHandler struct {
	client *BybitPrivateStreamClient
}

func (h *wsPrivateStreamHandler) OnOpen(socket *gws.Conn) {
	log().Info().Int("accountID", h.client.accountID).Msg("Bybit Private Stream connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
}

func (h *wsPrivateStreamHandler) OnClose(socket *gws.Conn, err error) {
	log().Info().Err(err).Int("accountID", h.client.accountID).Msg("Bybit Private Stream connection closed")
}

func (h *wsPrivateStreamHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsPrivateStreamHandler) OnPong(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
}

func (h *wsPrivateStreamHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
	h.client.processMessage(message.Bytes())
}

// ============================================================================
// BybitOrderEntryClient - WebSocket Order Entry
// ============================================================================

// BybitOrderEntryClient handles Bybit WebSocket order entry
// Endpoint: wss://stream.bybit.com/v5/trade
// Purpose: Submit, amend, cancel orders via WebSocket
// Methods: order.create, order.amend, order.cancel
type BybitOrderEntryClient struct {
	catalog   *catalog.Catalog
	msgBus    *msgbus.MsgBus
	accountID int
	account   cpanel.Account
	apiKey    cpanel.APIKey

	// WebSocket connection
	conn     *gws.Conn
	connLock sync.RWMutex

	// Request ID for tracking responses
	requestID atomic.Uint64

	// Pending order tracking: reqID -> clientOrderID
	pendingOrders map[uint64]int
	pendingMu     sync.Mutex

	// Connection state
	connected     atomic.Bool
	authenticated atomic.Bool
	shouldStop    atomic.Bool

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building messages
	msgBuffer bytes.Buffer
	bufLock   sync.Mutex
}

// NewBybitOrderEntryClient creates a new Bybit order entry client
func NewBybitOrderEntryClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus, accountID int, apiKeyName string) (*BybitOrderEntryClient, error) {
	account, err := catalog.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	apiKey, err := account.GetAPI(apiKeyName)
	if err != nil {
		return nil, err
	}

	return &BybitOrderEntryClient{
		catalog:       catalog,
		msgBus:        msgBus,
		accountID:     accountID,
		account:       *account,
		apiKey:        *apiKey,
		pendingOrders: make(map[uint64]int),
	}, nil
}

// Connect establishes the WebSocket connection for order entry
func (c *BybitOrderEntryClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	handler := &wsOrderEntryHandler{client: c}

	option := &gws.ClientOption{
		Addr:             WsTradeURL,
		ReadBufferSize:   wsExecReadBufferSize,
		CheckUtf8Enabled: false,
		NewDialer: func() (gws.Dialer, error) {
			return &ipv4Dialer{}, nil
		},
	}

	conn, _, err := gws.NewClient(handler, option)
	if err != nil {
		return err
	}

	c.connLock.Lock()
	c.conn = conn
	c.connLock.Unlock()
	c.connected.Store(true)

	// Start ReadLoop in a goroutine
	go func() {
		conn.ReadLoop()
		c.handleDisconnect()
	}()

	// Start ping loop
	go c.pingLoop()

	// Authenticate immediately after connection
	if err := c.authenticate(); err != nil {
		log().Error().Err(err).Msg("Failed to authenticate order entry")
		return err
	}

	log().Info().Int("accountID", c.accountID).Msg("Connected to Bybit Order Entry WebSocket")
	return nil
}

// Disconnect closes the WebSocket connection
func (c *BybitOrderEntryClient) Disconnect() {
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
	c.authenticated.Store(false)

	log().Info().Int("accountID", c.accountID).Msg("Disconnected from Bybit Order Entry WebSocket")
}

// SubmitOrder submits a new order via WebSocket
// Bybit order.create format:
//
//	{
//	  "reqId": "xxx",
//	  "header": {"X-BAPI-TIMESTAMP": "xxx", "X-BAPI-RECV-WINDOW": "5000"},
//	  "op": "order.create",
//	  "args": [{
//	    "category": "spot",
//	    "symbol": "BTCUSDT",
//	    "side": "Buy",
//	    "orderType": "Limit",
//	    "qty": "0.001",
//	    "price": "50000",
//	    "timeInForce": "GTC",
//	    "orderLinkId": "xxx"
//	  }]
//	}
func (c *BybitOrderEntryClient) SubmitOrder(symbolID int, clientOrderID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		return &HTTPError{StatusCode: 0, Body: "unsupported product type for Bybit"}
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	timestamp := time.Now().UnixMilli()
	reqID := c.requestID.Add(1)

	// Track pending order for response correlation
	c.pendingMu.Lock()
	c.pendingOrders[reqID] = clientOrderID
	c.pendingMu.Unlock()

	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"reqId":"`)
	c.msgBuffer.WriteString(strconv.FormatUint(reqID, 10))
	c.msgBuffer.WriteString(`","header":{"X-BAPI-TIMESTAMP":"`)
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString(`","X-BAPI-RECV-WINDOW":"`)
	c.msgBuffer.WriteString(strconv.Itoa(wsExecRecvWindow))
	c.msgBuffer.WriteString(`","X-BAPI-API-KEY":"`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`","X-BAPI-SIGN":"`)

	// Build signature payload
	signPayload := c.buildSignPayload(timestamp, reqID)
	signature := c.signHMAC(signPayload)
	c.msgBuffer.WriteString(signature)

	c.msgBuffer.WriteString(`"},"op":"order.create","args":[{"category":"`)
	c.msgBuffer.WriteString(string(category))
	c.msgBuffer.WriteString(`","symbol":"`)
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString(`","side":"`)
	if side == common.SideBuy {
		c.msgBuffer.WriteString("Buy")
	} else {
		c.msgBuffer.WriteString("Sell")
	}
	c.msgBuffer.WriteString(`","orderType":"`)
	if orderType == common.OrderTypeLimit {
		c.msgBuffer.WriteString("Limit")
	} else {
		c.msgBuffer.WriteString("Market")
	}
	c.msgBuffer.WriteString(`","qty":"`)
	c.msgBuffer.WriteString(strconv.FormatFloat(quantity, 'f', -1, 64))
	c.msgBuffer.WriteString(`"`)

	// Price only for limit orders
	if orderType == common.OrderTypeLimit {
		c.msgBuffer.WriteString(`,"price":"`)
		c.msgBuffer.WriteString(strconv.FormatFloat(price, 'f', -1, 64))
		c.msgBuffer.WriteString(`"`)
	}

	// Time in force
	c.msgBuffer.WriteString(`,"timeInForce":"`)
	switch timeInForce {
	case common.TimeInForceGTC:
		c.msgBuffer.WriteString("GTC")
	case common.TimeInForceIOC:
		c.msgBuffer.WriteString("IOC")
	case common.TimeInForceFOK:
		c.msgBuffer.WriteString("FOK")
	case common.TimeInForcePO:
		c.msgBuffer.WriteString("PostOnly")
	default:
		c.msgBuffer.WriteString("GTC")
	}
	c.msgBuffer.WriteString(`"`)

	// Client order ID (orderLinkId)
	c.msgBuffer.WriteString(`,"orderLinkId":"`)
	c.msgBuffer.WriteString(strconv.Itoa(clientOrderID))
	c.msgBuffer.WriteString(`"}]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// CancelOrder cancels an order by orderID via WebSocket
func (c *BybitOrderEntryClient) CancelOrder(symbolID int, orderID string) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		return &HTTPError{StatusCode: 0, Body: "unsupported product type for Bybit"}
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	timestamp := time.Now().UnixMilli()
	reqID := c.requestID.Add(1)

	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"reqId":"`)
	c.msgBuffer.WriteString(strconv.FormatUint(reqID, 10))
	c.msgBuffer.WriteString(`","header":{"X-BAPI-TIMESTAMP":"`)
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString(`","X-BAPI-RECV-WINDOW":"`)
	c.msgBuffer.WriteString(strconv.Itoa(wsExecRecvWindow))
	c.msgBuffer.WriteString(`","X-BAPI-API-KEY":"`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`","X-BAPI-SIGN":"`)

	signPayload := c.buildSignPayload(timestamp, reqID)
	signature := c.signHMAC(signPayload)
	c.msgBuffer.WriteString(signature)

	c.msgBuffer.WriteString(`"},"op":"order.cancel","args":[{"category":"`)
	c.msgBuffer.WriteString(string(category))
	c.msgBuffer.WriteString(`","symbol":"`)
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString(`","orderId":"`)
	c.msgBuffer.WriteString(orderID)
	c.msgBuffer.WriteString(`"}]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// CancelOrderByLinkID cancels an order by orderLinkId (clientOrderID) via WebSocket
func (c *BybitOrderEntryClient) CancelOrderByLinkID(symbolID int, orderLinkID string) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		return &HTTPError{StatusCode: 0, Body: "unsupported product type for Bybit"}
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	timestamp := time.Now().UnixMilli()
	reqID := c.requestID.Add(1)

	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"reqId":"`)
	c.msgBuffer.WriteString(strconv.FormatUint(reqID, 10))
	c.msgBuffer.WriteString(`","header":{"X-BAPI-TIMESTAMP":"`)
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString(`","X-BAPI-RECV-WINDOW":"`)
	c.msgBuffer.WriteString(strconv.Itoa(wsExecRecvWindow))
	c.msgBuffer.WriteString(`","X-BAPI-API-KEY":"`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`","X-BAPI-SIGN":"`)

	signPayload := c.buildSignPayload(timestamp, reqID)
	signature := c.signHMAC(signPayload)
	c.msgBuffer.WriteString(signature)

	c.msgBuffer.WriteString(`"},"op":"order.cancel","args":[{"category":"`)
	c.msgBuffer.WriteString(string(category))
	c.msgBuffer.WriteString(`","symbol":"`)
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString(`","orderLinkId":"`)
	c.msgBuffer.WriteString(orderLinkID)
	c.msgBuffer.WriteString(`"}]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// CancelAllOrders cancels all open orders for a symbol via WebSocket
func (c *BybitOrderEntryClient) CancelAllOrders(symbolID int) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	category, ok := ProductSlugToCategory[symbol.Product.Slug]
	if !ok {
		return &HTTPError{StatusCode: 0, Body: "unsupported product type for Bybit"}
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	timestamp := time.Now().UnixMilli()
	reqID := c.requestID.Add(1)

	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"reqId":"`)
	c.msgBuffer.WriteString(strconv.FormatUint(reqID, 10))
	c.msgBuffer.WriteString(`","header":{"X-BAPI-TIMESTAMP":"`)
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString(`","X-BAPI-RECV-WINDOW":"`)
	c.msgBuffer.WriteString(strconv.Itoa(wsExecRecvWindow))
	c.msgBuffer.WriteString(`","X-BAPI-API-KEY":"`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`","X-BAPI-SIGN":"`)

	signPayload := c.buildSignPayload(timestamp, reqID)
	signature := c.signHMAC(signPayload)
	c.msgBuffer.WriteString(signature)

	c.msgBuffer.WriteString(`"},"op":"order.cancelAll","args":[{"category":"`)
	c.msgBuffer.WriteString(string(category))
	c.msgBuffer.WriteString(`","symbol":"`)
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString(`"}]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// authenticate sends authentication message for order entry
func (c *BybitOrderEntryClient) authenticate() error {
	expires := time.Now().UnixMilli() + wsExecAuthExpiry*1000

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build signature payload: "GET/realtime" + expires
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString("GET/realtime")
	c.msgBuffer.WriteString(strconv.FormatInt(expires, 10))

	signature := c.signHMAC(c.msgBuffer.Bytes())

	// Build auth message
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString(`{"op":"auth","args":["`)
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString(`",`)
	c.msgBuffer.WriteString(strconv.FormatInt(expires, 10))
	c.msgBuffer.WriteString(`,"`)
	c.msgBuffer.WriteString(signature)
	c.msgBuffer.WriteString(`"]}`)

	return c.sendMessage(c.msgBuffer.Bytes())
}

// buildSignPayload builds the signature payload for order requests
// Format: timestamp + api_key + recv_window + request_body_json
func (c *BybitOrderEntryClient) buildSignPayload(timestamp int64, reqID uint64) []byte {
	var buf bytes.Buffer
	buf.WriteString(strconv.FormatInt(timestamp, 10))
	buf.WriteString(c.apiKey.Key)
	buf.WriteString(strconv.Itoa(wsExecRecvWindow))
	buf.WriteString("req_id=")
	buf.WriteString(strconv.FormatUint(reqID, 10))
	return buf.Bytes()
}

// signHMAC signs the payload with HMAC-SHA256
func (c *BybitOrderEntryClient) signHMAC(payload []byte) string {
	h := hmac.New(sha256.New, []byte(c.apiKey.Secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// sendMessage sends a WebSocket message
func (c *BybitOrderEntryClient) sendMessage(msg []byte) error {
	c.connLock.RLock()
	conn := c.conn
	c.connLock.RUnlock()

	if conn == nil {
		return &HTTPError{StatusCode: 0, Body: "WebSocket not connected"}
	}

	return conn.WriteMessage(gws.OpcodeText, msg)
}

// pingLoop sends periodic pings to keep the connection alive
func (c *BybitOrderEntryClient) pingLoop() {
	ticker := time.NewTicker(wsExecPingInterval)
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
				if err := conn.WriteMessage(gws.OpcodeText, []byte(`{"op":"ping"}`)); err != nil {
					log().Warn().Err(err).Msg("Failed to send ping to order entry")
				}
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *BybitOrderEntryClient) handleDisconnect() {
	c.connected.Store(false)
	c.authenticated.Store(false)

	if c.shouldStop.Load() {
		return
	}

	log().Warn().Int("accountID", c.accountID).Msg("Order entry disconnected, attempting to reconnect...")

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.Connect(c.ctx); err != nil {
			log().Error().Err(err).Msg("Order entry reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes incoming WebSocket messages (order responses)
func (c *BybitOrderEntryClient) processMessage(data []byte) {
	// Check for op response
	op, err := jsonparser.GetString(data, "op")
	if err == nil {
		c.processOpResponse(op, data)
		return
	}

	// Check for retCode in response
	retCode, err := jsonparser.GetInt(data, "retCode")
	if err == nil && retCode != 0 {
		retMsg, _ := jsonparser.GetString(data, "retMsg")
		reqID, _ := jsonparser.GetString(data, "reqId")
		log().Error().
			Int64("retCode", retCode).
			Str("retMsg", retMsg).
			Str("reqId", reqID).
			Msg("Order entry request failed")
	}
}

// processOpResponse handles operation responses
func (c *BybitOrderEntryClient) processOpResponse(op string, data []byte) {
	switch op {
	case "pong":
		return
	case "auth":
		// Trade websocket returns retCode:0 for success, not success:true
		retCode, _ := jsonparser.GetInt(data, "retCode")
		if retCode == 0 {
			c.authenticated.Store(true)
			log().Info().Int("accountID", c.accountID).Msg("Order entry authenticated")
		} else {
			retMsg, _ := jsonparser.GetString(data, "retMsg")
			apiKey := c.apiKey.Key
			log().Error().
				Str("msg", retMsg).
				Int64("retCode", retCode).
				Int("accountID", c.accountID).
				Str("accountName", c.account.Name).
				Str("apiKey", apiKey).
				Msg("Order entry authentication failed")
		}
	case "order.create", "order.cancel", "order.cancelAll":
		retCode, _ := jsonparser.GetInt(data, "retCode")
		reqIDStr, _ := jsonparser.GetString(data, "reqId")
		reqID, _ := strconv.ParseUint(reqIDStr, 10, 64)

		// Look up and remove the pending order
		c.pendingMu.Lock()
		clientOrderID, found := c.pendingOrders[reqID]
		delete(c.pendingOrders, reqID)
		c.pendingMu.Unlock()

		if retCode != 0 {
			retMsg, _ := jsonparser.GetString(data, "retMsg")
			apiKey := c.apiKey.Key
			maskedKey := apiKey
			if len(apiKey) > 6 {
				maskedKey = apiKey[:3] + "***" + apiKey[len(apiKey)-3:]
			}
			log().Error().
				Str("op", op).
				Int64("retCode", retCode).
				Str("retMsg", retMsg).
				Int("accountID", c.accountID).
				Str("accountName", c.account.Name).
				Str("apiKey", maskedKey).
				Int("clientOrderID", clientOrderID).
				Msg("Order operation failed")

			// Publish OrderRejected event so strategy is notified (no exchange timestamp; use received time)
			if found && op == "order.create" {
				ev := event.OrderRejected{
					ClientOrderID: clientOrderID,
					OrderID:       -1,
					AccountID:     c.accountID,
					ErrorCode:     int(retCode),
					UpdatedAt:     uint64(time.Now().UnixNano()),
					Msg:           retMsg,
				}
				offset, buf := c.msgBus.Allocate(uint64(ev.GetBufferLength()))
				ev.Encode(buf)
				c.msgBus.Publish(msgbus.EventRef{
					Topic:  event.TopicEventOrderRejected,
					Index:  offset,
					Length: uint64(ev.GetBufferLength()),
				})
			}
		}
	}
}

// wsOrderEntryHandler implements gws.Event interface for order entry WebSocket
type wsOrderEntryHandler struct {
	client *BybitOrderEntryClient
}

func (h *wsOrderEntryHandler) OnOpen(socket *gws.Conn) {
	log().Info().Int("accountID", h.client.accountID).Msg("Bybit Order Entry connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
}

func (h *wsOrderEntryHandler) OnClose(socket *gws.Conn, err error) {
	log().Info().Err(err).Int("accountID", h.client.accountID).Msg("Bybit Order Entry connection closed")
}

func (h *wsOrderEntryHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsOrderEntryHandler) OnPong(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
}

func (h *wsOrderEntryHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	_ = socket.SetDeadline(time.Now().Add(wsExecPingInterval + wsExecPingWait))
	h.client.processMessage(message.Bytes())
}

// ============================================================================
// BybitExecutionClient - Unified Wrapper for ExecutionClient Interface
// ============================================================================

// BybitExecutionClient wraps BybitPrivateStreamClient and BybitOrderEntryClient
// to implement the ExecutionClient interface for use with ExecutionRouter.
// It coordinates between the two underlying clients:
// - BybitPrivateStreamClient: for receiving order updates, executions, and wallet updates
// - BybitOrderEntryClient: for sending orders via WebSocket
// - BybitHTTPClient: for HTTP requests (e.g., balance snapshots)
type BybitExecutionClient struct {
	privateStream *BybitPrivateStreamClient
	orderEntry    *BybitOrderEntryClient
	httpClient    *BybitHTTPClient

	// Account info for HTTP requests
	accountID  int
	apiKeyName string
}

// NewBybitExecutionClient creates a new Bybit execution client that wraps
// both the private stream and order entry clients
func NewBybitExecutionClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus, accountID int, apiKeyName string) (*BybitExecutionClient, error) {
	privateStream, err := NewBybitPrivateStreamClient(catalog, msgBus, accountID, apiKeyName)
	if err != nil {
		return nil, err
	}

	orderEntry, err := NewBybitOrderEntryClient(catalog, msgBus, accountID, apiKeyName)
	if err != nil {
		return nil, err
	}

	httpClient := NewBybitHTTPClient(catalog, msgBus)

	return &BybitExecutionClient{
		privateStream: privateStream,
		orderEntry:    orderEntry,
		httpClient:    &httpClient,
		accountID:     accountID,
		apiKeyName:    apiKeyName,
	}, nil
}

// Connect establishes connections for both private stream and order entry
func (c *BybitExecutionClient) Connect(ctx context.Context) error {
	// Connect private stream first (for receiving updates)
	if err := c.privateStream.Connect(ctx); err != nil {
		return err
	}

	// Connect order entry (for sending orders)
	if err := c.orderEntry.Connect(ctx); err != nil {
		// Disconnect private stream if order entry fails
		c.privateStream.Disconnect()
		return err
	}

	return nil
}

// Disconnect closes connections for both private stream and order entry
func (c *BybitExecutionClient) Disconnect() {
	c.orderEntry.Disconnect()
	c.privateStream.Disconnect()
}

// SubscribeOrderUpdate subscribes to order status update events
// Bybit: Subscribes to "order" topic on private stream
func (c *BybitExecutionClient) SubscribeOrderUpdate() error {
	return c.privateStream.Subscribe([]string{"order"})
}

// SubscribeFill subscribes to execution/fill events
// Bybit: Subscribes to "execution" topic on private stream
func (c *BybitExecutionClient) SubscribeFill() error {
	return c.privateStream.Subscribe([]string{"execution"})
}

// SubscribeBalance subscribes to wallet/balance update events
// Bybit: Subscribes to "wallet" topic on private stream
func (c *BybitExecutionClient) SubscribeBalance() error {
	return c.privateStream.Subscribe([]string{"wallet"})
}

// SubmitOrder submits a new order via the order entry WebSocket
// SubmitOrder submits a new order with the strategy-provided clientOrderID
func (c *BybitExecutionClient) SubmitOrder(clientOrderID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	return c.orderEntry.SubmitOrder(symbolID, clientOrderID, side, orderType, timeInForce, price, quantity)
}

// CancelOrder cancels an order by orderID via the order entry WebSocket
// Note: Bybit uses string orderIDs, so we convert int to string
func (c *BybitExecutionClient) CancelOrder(symbolID int, orderID int) error {
	return c.orderEntry.CancelOrder(symbolID, strconv.Itoa(orderID))
}

// CancelAllOrders cancels all open orders for a symbol via the order entry WebSocket
func (c *BybitExecutionClient) CancelAllOrders(symbolID int) error {
	return c.orderEntry.CancelAllOrders(symbolID)
}

// ReqBalanceSnapshot requests the current balance snapshot via HTTP API
// The response will be published as a ReqBalanceSnapshot event
func (c *BybitExecutionClient) ReqBalanceSnapshot() error {
	// Use UNIFIED account type for Bybit unified margin
	return c.httpClient.ReqBalanceSnapshot(c.accountID, c.apiKeyName, "UNIFIED")
}

// PrivateStream returns the underlying private stream client for advanced usage
func (c *BybitExecutionClient) PrivateStream() *BybitPrivateStreamClient {
	return c.privateStream
}

// OrderEntry returns the underlying order entry client for advanced usage
func (c *BybitExecutionClient) OrderEntry() *BybitOrderEntryClient {
	return c.orderEntry
}
