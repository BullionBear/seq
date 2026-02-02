package binance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/buger/jsonparser"
	"github.com/lxzan/gws"
)

const (
	// WebSocket API connection settings
	wsAPIReadBufferSize = 64 * 1024 // 64KB read buffer
	wsAPIPingInterval   = 30 * time.Second
	wsAPIPingWait       = 10 * time.Second
	wsAPIRecvWindow     = 5000 // 5 seconds
)

// BinanceSpotExecutionClient handles Binance spot order execution via WebSocket API
// It uses Ed25519 API key for signing and supports order submission, cancellation,
// and receiving order updates/fills via the WebSocket connection.
type BinanceSpotExecutionClient struct {
	catalog   *catalog.Catalog
	eventBus  *evbus.EventBus
	accountID int
	account   cpanel.Account

	// Ed25519 private key for signing
	privateKey ed25519.PrivateKey

	// WebSocket connection for trading
	conn     *gws.Conn
	connLock sync.RWMutex

	// Request ID for WebSocket requests
	requestID atomic.Uint64

	// Connection state
	connected  atomic.Bool
	shouldStop atomic.Bool

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building messages
	msgBuffer bytes.Buffer
	bufLock   sync.Mutex
}

// NewBinanceSpotExecutionClient creates a new Binance spot execution client
func NewBinanceSpotExecutionClient(catalog *catalog.Catalog, eventBus *evbus.EventBus, accountID int) (*BinanceSpotExecutionClient, error) {
	account, err := catalog.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	// Parse Ed25519 private key (supports PEM format or raw base64)
	privateKey, err := parseEd25519PrivateKey(account.APISecret)
	if err != nil {
		return nil, err
	}

	return &BinanceSpotExecutionClient{
		catalog:    catalog,
		eventBus:   eventBus,
		accountID:  accountID,
		account:    *account,
		privateKey: privateKey,
	}, nil
}

// parseEd25519PrivateKey parses an Ed25519 private key from various formats:
// - PEM encoded PKCS8 format (-----BEGIN PRIVATE KEY-----)
// - Base64 encoded raw seed (32 bytes) or full private key (64 bytes)
func parseEd25519PrivateKey(secret string) (ed25519.PrivateKey, error) {
	// Try PEM format first
	block, _ := pem.Decode([]byte(secret))
	if block != nil {
		// Parse PKCS8 private key
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		ed25519Key, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, &HTTPError{StatusCode: 0, Body: "PEM key is not Ed25519"}
		}
		return ed25519Key, nil
	}

	// Try base64 encoded raw key
	privateKeyBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}

	// Ed25519 private key should be 64 bytes (seed + public key) or 32 bytes (seed only)
	if len(privateKeyBytes) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(privateKeyBytes), nil
	} else if len(privateKeyBytes) == ed25519.PrivateKeySize {
		return privateKeyBytes, nil
	}

	return nil, &HTTPError{StatusCode: 0, Body: "invalid Ed25519 private key size"}
}

// Connect establishes the WebSocket connection for trading
func (c *BinanceSpotExecutionClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.shouldStop.Store(false)

	handler := &wsExecEventHandler{client: c}

	option := &gws.ClientOption{
		Addr:             BaseWsAPIURL,
		ReadBufferSize:   wsAPIReadBufferSize,
		CheckUtf8Enabled: false,
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

	log().Info().Int("accountID", c.accountID).Msg("Connected to Binance WebSocket API for trading")
	return nil
}

// Disconnect closes the WebSocket connection
func (c *BinanceSpotExecutionClient) Disconnect() {
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

	log().Info().Int("accountID", c.accountID).Msg("Disconnected from Binance WebSocket API")
}

// SubmitOrder submits a new order via WebSocket
func (c *BinanceSpotExecutionClient) SubmitOrder(symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing
	c.msgBuffer.Reset()

	// symbol
	c.msgBuffer.WriteString("symbol=")
	c.msgBuffer.WriteString(symbol.Name)

	// side
	c.msgBuffer.WriteString("&side=")
	switch side {
	case common.SideBuy:
		c.msgBuffer.WriteString("BUY")
	case common.SideSell:
		c.msgBuffer.WriteString("SELL")
	}

	// type
	c.msgBuffer.WriteString("&type=")
	isLimitMaker := false
	switch orderType {
	case common.OrderTypeLimit:
		if timeInForce == common.TimeInForcePO {
			c.msgBuffer.WriteString("LIMIT_MAKER")
			isLimitMaker = true
		} else {
			c.msgBuffer.WriteString("LIMIT")
		}
	case common.OrderTypeMarket:
		c.msgBuffer.WriteString("MARKET")
	}

	// timeInForce
	if orderType == common.OrderTypeLimit && !isLimitMaker {
		c.msgBuffer.WriteString("&timeInForce=")
		switch timeInForce {
		case common.TimeInForceGTC:
			c.msgBuffer.WriteString("GTC")
		case common.TimeInForceIOC:
			c.msgBuffer.WriteString("IOC")
		case common.TimeInForceFOK:
			c.msgBuffer.WriteString("FOK")
		}
	}

	// quantity
	c.msgBuffer.WriteString("&quantity=")
	c.msgBuffer.WriteString(strconv.FormatFloat(quantity, 'f', -1, 64))

	// price
	if orderType == common.OrderTypeLimit {
		c.msgBuffer.WriteString("&price=")
		c.msgBuffer.WriteString(strconv.FormatFloat(price, 'f', -1, 64))
	}

	// timestamp
	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	// recvWindow
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))

	// apiKey (required for signature)
	c.msgBuffer.WriteString("&apiKey=")
	c.msgBuffer.WriteString(c.account.APIKey)

	// Sign with Ed25519
	signature := c.signEd25519(c.msgBuffer.Bytes())

	// Build WebSocket request message
	requestID := c.requestID.Add(1)
	msg := c.buildOrderNewRequest(symbol.Name, side, orderType, timeInForce, price, quantity, timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// CancelOrder cancels an order by orderID via WebSocket
func (c *BinanceSpotExecutionClient) CancelOrder(symbolID int, orderID int) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString("symbol=")
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString("&orderId=")
	c.msgBuffer.WriteString(strconv.Itoa(orderID))

	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&apiKey=")
	c.msgBuffer.WriteString(c.account.APIKey)

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildOrderCancelRequest(symbol.Name, orderID, timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// CancelAllOrders cancels all open orders for a symbol via WebSocket
func (c *BinanceSpotExecutionClient) CancelAllOrders(symbolID int) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing
	c.msgBuffer.Reset()
	c.msgBuffer.WriteString("symbol=")
	c.msgBuffer.WriteString(symbol.Name)

	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&apiKey=")
	c.msgBuffer.WriteString(c.account.APIKey)

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildCancelAllOrdersRequest(symbol.Name, timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// ReqBalanceSnapshot requests account balance snapshot via WebSocket
// The response will be published as a ReqBalanceSnapshot event to the event bus
func (c *BinanceSpotExecutionClient) ReqBalanceSnapshot() error {
	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing (same order as JSON params, including apiKey but not signature)
	c.msgBuffer.Reset()

	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.account.APIKey)
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildAccountStatusRequest(timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// buildAccountStatusRequest builds a JSON message for account.status
func (c *BinanceSpotExecutionClient) buildAccountStatusRequest(timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(256)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"account.status","params":{`)

	buf.WriteString(`"apiKey":"`)
	buf.WriteString(c.account.APIKey)
	buf.WriteString(`"`)

	buf.WriteString(`,"recvWindow":`)
	buf.WriteString(strconv.Itoa(wsAPIRecvWindow))

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"signature":"`)
	buf.WriteString(signature)
	buf.WriteString(`"`)

	buf.WriteString(`}}`)

	return buf.Bytes()
}

// signEd25519 signs the payload with Ed25519 and returns base64-encoded signature
// Binance expects standard base64 encoding for Ed25519 signatures
func (c *BinanceSpotExecutionClient) signEd25519(payload []byte) string {
	signature := ed25519.Sign(c.privateKey, payload)
	return base64.StdEncoding.EncodeToString(signature)
}

// buildOrderNewRequest builds a JSON message for order.place
func (c *BinanceSpotExecutionClient) buildOrderNewRequest(symbol string, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64, timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(512)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"order.place","params":{`)

	buf.WriteString(`"symbol":"`)
	buf.WriteString(symbol)
	buf.WriteString(`"`)

	buf.WriteString(`,"side":"`)
	if side == common.SideBuy {
		buf.WriteString("BUY")
	} else {
		buf.WriteString("SELL")
	}
	buf.WriteString(`"`)

	buf.WriteString(`,"type":"`)
	isLimitMaker := false
	switch orderType {
	case common.OrderTypeLimit:
		if timeInForce == common.TimeInForcePO {
			buf.WriteString("LIMIT_MAKER")
			isLimitMaker = true
		} else {
			buf.WriteString("LIMIT")
		}
	case common.OrderTypeMarket:
		buf.WriteString("MARKET")
	}
	buf.WriteString(`"`)

	if orderType == common.OrderTypeLimit && !isLimitMaker {
		buf.WriteString(`,"timeInForce":"`)
		switch timeInForce {
		case common.TimeInForceGTC:
			buf.WriteString("GTC")
		case common.TimeInForceIOC:
			buf.WriteString("IOC")
		case common.TimeInForceFOK:
			buf.WriteString("FOK")
		}
		buf.WriteString(`"`)
	}

	buf.WriteString(`,"quantity":"`)
	buf.WriteString(strconv.FormatFloat(quantity, 'f', -1, 64))
	buf.WriteString(`"`)

	if orderType == common.OrderTypeLimit {
		buf.WriteString(`,"price":"`)
		buf.WriteString(strconv.FormatFloat(price, 'f', -1, 64))
		buf.WriteString(`"`)
	}

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"recvWindow":`)
	buf.WriteString(strconv.Itoa(wsAPIRecvWindow))

	buf.WriteString(`,"apiKey":"`)
	buf.WriteString(c.account.APIKey)
	buf.WriteString(`"`)

	buf.WriteString(`,"signature":"`)
	buf.WriteString(signature)
	buf.WriteString(`"`)

	buf.WriteString(`}}`)

	return buf.Bytes()
}

// buildOrderCancelRequest builds a JSON message for order.cancel
func (c *BinanceSpotExecutionClient) buildOrderCancelRequest(symbol string, orderID int, timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(256)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"order.cancel","params":{`)

	buf.WriteString(`"symbol":"`)
	buf.WriteString(symbol)
	buf.WriteString(`"`)

	buf.WriteString(`,"orderId":`)
	buf.WriteString(strconv.Itoa(orderID))

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"recvWindow":`)
	buf.WriteString(strconv.Itoa(wsAPIRecvWindow))

	buf.WriteString(`,"apiKey":"`)
	buf.WriteString(c.account.APIKey)
	buf.WriteString(`"`)

	buf.WriteString(`,"signature":"`)
	buf.WriteString(signature)
	buf.WriteString(`"`)

	buf.WriteString(`}}`)

	return buf.Bytes()
}

// buildCancelAllOrdersRequest builds a JSON message for openOrders.cancelAll
func (c *BinanceSpotExecutionClient) buildCancelAllOrdersRequest(symbol string, timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(256)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"openOrders.cancelAll","params":{`)

	buf.WriteString(`"symbol":"`)
	buf.WriteString(symbol)
	buf.WriteString(`"`)

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"recvWindow":`)
	buf.WriteString(strconv.Itoa(wsAPIRecvWindow))

	buf.WriteString(`,"apiKey":"`)
	buf.WriteString(c.account.APIKey)
	buf.WriteString(`"`)

	buf.WriteString(`,"signature":"`)
	buf.WriteString(signature)
	buf.WriteString(`"`)

	buf.WriteString(`}}`)

	return buf.Bytes()
}

// sendMessage sends a WebSocket message
func (c *BinanceSpotExecutionClient) sendMessage(msg []byte) error {
	c.connLock.RLock()
	conn := c.conn
	c.connLock.RUnlock()

	if conn == nil {
		return &HTTPError{StatusCode: 0, Body: "WebSocket not connected"}
	}

	return conn.WriteMessage(gws.OpcodeText, msg)
}

// pingLoop sends periodic pings to keep the connection alive
func (c *BinanceSpotExecutionClient) pingLoop() {
	ticker := time.NewTicker(wsAPIPingInterval)
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
					log().Warn().Err(err).Msg("Failed to send ping to WebSocket API")
				}
			}
		}
	}
}

// handleDisconnect handles WebSocket disconnection
func (c *BinanceSpotExecutionClient) handleDisconnect() {
	c.connected.Store(false)

	if c.shouldStop.Load() {
		return
	}

	log().Warn().Int("accountID", c.accountID).Msg("WebSocket API disconnected, attempting to reconnect...")

	for !c.shouldStop.Load() {
		time.Sleep(wsReconnectInterval)

		if err := c.Connect(c.ctx); err != nil {
			log().Error().Err(err).Msg("WebSocket API reconnection failed")
			continue
		}
		break
	}
}

// processMessage processes incoming WebSocket messages
func (c *BinanceSpotExecutionClient) processMessage(data []byte) {
	// Check for error
	errCode, err := jsonparser.GetInt(data, "error", "code")
	if err == nil && errCode != 0 {
		errMsg, _ := jsonparser.GetString(data, "error", "msg")
		log().Error().Int64("code", errCode).Str("msg", errMsg).Msg("WebSocket API error")
		return
	}

	// Check for result
	result, _, _, err := jsonparser.Get(data, "result")
	if err == nil && len(result) > 0 {
		// Check if this is an account status response (has "balances" field)
		_, _, _, balancesErr := jsonparser.Get(result, "balances")
		if balancesErr == nil {
			c.processAccountStatusResponse(result)
			return
		}

		// Otherwise, try to process as order response
		c.processOrderResponse(result)
	}
}

// processOrderResponse processes order-related responses
func (c *BinanceSpotExecutionClient) processOrderResponse(data []byte) {
	// Parse order response fields
	orderIDInt, err := jsonparser.GetInt(data, "orderId")
	if err != nil {
		return // Not an order response
	}

	clientOrderIDStr, _ := jsonparser.GetString(data, "clientOrderId")
	clientOrderID, _ := strconv.Atoi(clientOrderIDStr)

	status, _ := jsonparser.GetString(data, "status")
	executedQtyStr, _ := jsonparser.GetString(data, "executedQty")
	executedQty := parseFloat64([]byte(executedQtyStr))

	updateTime, _ := jsonparser.GetInt(data, "transactTime")
	if updateTime == 0 {
		updateTime, _ = jsonparser.GetInt(data, "updateTime")
	}

	// Create OrderUpdate event
	orderUpdate := event.OrderUpdate{
		ClientOrderID: clientOrderID,
		OrderID:       int(orderIDInt),
		OrderStatus:   c.parseOrderStatus(status),
		ExecutedQty:   executedQty,
		UpdatedAt:     uint64(updateTime) * 1_000_000, // Convert ms to ns
	}

	// Publish to event bus
	size := evbus.OrderUpdateSize()
	offset, buf := c.eventBus.Allocate(size)
	evbus.SerializeOrderUpdate(buf, &orderUpdate)
	c.eventBus.Publish(evbus.EventRef{
		Topic:  event.TopicEventOrderUpdate,
		Index:  offset,
		Length: size,
	})

	// Check for fills in the response
	_, _, _, err = jsonparser.Get(data, "fills")
	if err == nil {
		c.processFills(data, clientOrderID, int(orderIDInt))
	}
}

// processFills processes fill information from order responses
func (c *BinanceSpotExecutionClient) processFills(data []byte, clientOrderID int, orderID int) {
	fillIdx := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		priceStr, _ := jsonparser.GetString(value, "price")
		qtyStr, _ := jsonparser.GetString(value, "qty")
		commissionStr, _ := jsonparser.GetString(value, "commission")
		commissionAsset, _ := jsonparser.GetString(value, "commissionAsset")
		tradeID, _ := jsonparser.GetInt(value, "tradeId")

		fill := event.Fill{
			ClientOrderID: clientOrderID,
			OrderID:       orderID,
			FillID:        int(tradeID),
			FilledQty:     parseFloat64([]byte(qtyStr)),
			FilledPrice:   parseFloat64([]byte(priceStr)),
			FeeCcyID:      c.getTokenID(commissionAsset),
			FeeQty:        parseFloat64([]byte(commissionStr)),
			FilledAt:      uint64(time.Now().UnixNano()),
		}

		// Publish fill event
		size := evbus.FillSize()
		fillOffset, buf := c.eventBus.Allocate(size)
		evbus.SerializeFill(buf, &fill)
		c.eventBus.Publish(evbus.EventRef{
			Topic:  event.TopicEventFill,
			Index:  fillOffset,
			Length: size,
		})

		fillIdx++
	}, "fills")
}

// processAccountStatusResponse processes account.status response and publishes ReqBalanceSnapshot
// Binance account.status response format:
//
//	{
//	  "makerCommission": 15,
//	  "takerCommission": 15,
//	  "buyerCommission": 0,
//	  "sellerCommission": 0,
//	  "canTrade": true,
//	  "canWithdraw": true,
//	  "canDeposit": true,
//	  "updateTime": 123456789,
//	  "accountType": "SPOT",
//	  "balances": [
//	    {"asset": "BTC", "free": "4723846.89208129", "locked": "0.00000000"},
//	    {"asset": "ETH", "free": "0.00000000", "locked": "0.00000000"}
//	  ]
//	}
func (c *BinanceSpotExecutionClient) processAccountStatusResponse(data []byte) {
	// Count non-zero balances first
	balanceCount := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		freeStr, _ := jsonparser.GetString(value, "free")
		lockedStr, _ := jsonparser.GetString(value, "locked")
		free := parseFloat64([]byte(freeStr))
		locked := parseFloat64([]byte(lockedStr))
		if free > 0 || locked > 0 {
			balanceCount++
		}
	}, "balances")

	if balanceCount == 0 {
		log().Debug().Msg("No non-zero balances found in account status response")
		return
	}

	// Allocate balances slice
	balances := make([]event.Balance, 0, balanceCount)

	// Parse balances
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		asset, _ := jsonparser.GetString(value, "asset")
		freeStr, _ := jsonparser.GetString(value, "free")
		lockedStr, _ := jsonparser.GetString(value, "locked")

		free := parseFloat64([]byte(freeStr))
		locked := parseFloat64([]byte(lockedStr))
		total := free + locked

		// Only include non-zero balances
		if total > 0 {
			balances = append(balances, event.Balance{
				TokenID:   c.getTokenID(asset),
				Available: free,
				Locked:    locked,
				Total:     total,
			})
		}
	}, "balances")

	// Create and publish ReqBalanceSnapshot event
	snapshot := event.ReqBalanceSnapshot{
		AccountID: c.accountID,
		Balances:  balances,
	}

	size := evbus.ReqBalanceSnapshotSize(&snapshot)
	offset, buf := c.eventBus.Allocate(size)
	evbus.SerializeReqBalanceSnapshot(buf, &snapshot)
	c.eventBus.Publish(evbus.EventRef{
		Topic:  event.TopicEventReqBalanceSnapshot,
		Index:  offset,
		Length: size,
	})

	log().Debug().Int("accountID", c.accountID).Int("balanceCount", len(balances)).Msg("Published balance snapshot")
}

// parseOrderStatus converts Binance order status string to common.OrderStatus
func (c *BinanceSpotExecutionClient) parseOrderStatus(status string) common.OrderStatus {
	switch strings.ToUpper(status) {
	case "NEW":
		return common.OrderStatusAccepted
	case "PARTIALLY_FILLED":
		return common.OrderStatusPartiallyFilled
	case "FILLED":
		return common.OrderStatusFilled
	case "CANCELED":
		return common.OrderStatusCanceled
	case "REJECTED":
		return common.OrderStatusRejected
	case "EXPIRED":
		return common.OrderStatusCanceled
	default:
		return common.OrderStatusUninitialized
	}
}

// getTokenID gets the token ID from the catalog by asset name
func (c *BinanceSpotExecutionClient) getTokenID(asset string) int {
	// This would typically look up the token ID from the catalog
	// For now, return 0 as a placeholder
	return 0
}

// wsExecEventHandler implements gws.Event interface for execution WebSocket
type wsExecEventHandler struct {
	client *BinanceSpotExecutionClient
}

func (h *wsExecEventHandler) OnOpen(socket *gws.Conn) {
	log().Info().Int("accountID", h.client.accountID).Msg("Binance WebSocket API connection opened")
	_ = socket.SetDeadline(time.Now().Add(wsAPIPingInterval + wsAPIPingWait))
}

func (h *wsExecEventHandler) OnClose(socket *gws.Conn, err error) {
	log().Info().Err(err).Int("accountID", h.client.accountID).Msg("Binance WebSocket API connection closed")
}

func (h *wsExecEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsAPIPingInterval + wsAPIPingWait))
	_ = socket.WritePong(payload)
}

func (h *wsExecEventHandler) OnPong(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(wsAPIPingInterval + wsAPIPingWait))
}

func (h *wsExecEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	_ = socket.SetDeadline(time.Now().Add(wsAPIPingInterval + wsAPIPingWait))
	h.client.processMessage(message.Bytes())
}
