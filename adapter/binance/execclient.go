package binance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
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
//
// Subscription Model:
// Binance only supports subscribing to the entire user data stream at once.
// When any granular subscription method is called (SubscribeOrderUpdate, SubscribeFill,
// SubscribeBalance), the client subscribes to the full stream internally but only
// publishes events for subscribed types. Unsubscribed event types are logged as unhandled.
type BinanceSpotExecutionClient struct {
	catalog   *catalog.Catalog
	msgBus    *msgbus.MsgBus
	accountID int
	walletID  int
	account   cpanel.Account
	apiKey    cpanel.APIKey

	// Ed25519 private key for signing
	privateKey ed25519.PrivateKey

	// WebSocket connection for trading
	conn     *gws.Conn
	connLock sync.RWMutex

	// Request ID for WebSocket requests
	requestID atomic.Uint64

	// Pending order.place requests: requestID -> clientOrderID (for publishing OrderRejected on API error)
	pendingOrderRequests sync.Map

	// Pending order.cancel requests: requestID -> clientOrderID (for publishing OrderCanceled on API error)
	pendingCancelRequests sync.Map

	// Connection state
	connected  atomic.Bool
	shouldStop atomic.Bool

	// Subscription state tracking
	// Binance subscribes to entire user data stream, but we filter events based on these flags
	userDataStreamSubscribed atomic.Bool // true if user data stream is subscribed
	subOrderUpdate           atomic.Bool // true if order update events should be published
	subFill                  atomic.Bool // true if fill events should be published
	subBalance               atomic.Bool // true if balance events should be published

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Pre-allocated buffer for building messages
	msgBuffer bytes.Buffer
	bufLock   sync.Mutex
}

// NewBinanceSpotExecutionClient creates a new Binance spot execution client
func NewBinanceSpotExecutionClient(catalog *catalog.Catalog, msgBus *msgbus.MsgBus, accountID int, apiKeyName string, walletID int) (*BinanceSpotExecutionClient, error) {
	account, err := catalog.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	apiKey, err := account.GetAPI(apiKeyName)
	if err != nil {
		return nil, err
	}

	// Parse Ed25519 private key (supports PEM format or raw base64)
	privateKey, err := parseEd25519PrivateKey(apiKey.Secret)
	if err != nil {
		return nil, err
	}

	return &BinanceSpotExecutionClient{
		catalog:    catalog,
		msgBus:     msgBus,
		accountID:  accountID,
		walletID:   walletID,
		account:    *account,
		apiKey:     *apiKey,
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

	// Subscribe to user data stream if any subscriptions were registered before Connect
	if c.hasAnySubscription() {
		if err := c.ensureUserDataStreamSubscribed(); err != nil {
			log().Error().Err(err).Msg("Failed to subscribe to user data stream")
			return err
		}
		log().Debug().Int("accountID", c.accountID).Msg("Subscribed to pending user data stream")
	}

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
func (c *BinanceSpotExecutionClient) SubmitOrder(clientOrderID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing (sorted alphabetically per Binance spec)
	c.msgBuffer.Reset()
	timestamp := time.Now().UnixMilli()

	isLimitMaker := orderType == common.OrderTypeLimit && timeInForce == common.TimeInForcePO

	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.apiKey.Key)

	c.msgBuffer.WriteString("&newClientOrderId=")
	c.msgBuffer.WriteString(strconv.Itoa(clientOrderID))

	if orderType == common.OrderTypeLimit {
		c.msgBuffer.WriteString("&price=")
		c.msgBuffer.WriteString(strconv.FormatFloat(price, 'f', -1, 64))
	}

	c.msgBuffer.WriteString("&quantity=")
	c.msgBuffer.WriteString(strconv.FormatFloat(quantity, 'f', -1, 64))

	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))

	c.msgBuffer.WriteString("&side=")
	switch side {
	case common.SideBuy:
		c.msgBuffer.WriteString("BUY")
	case common.SideSell:
		c.msgBuffer.WriteString("SELL")
	}

	c.msgBuffer.WriteString("&symbol=")
	c.msgBuffer.WriteString(symbol.Name)

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

	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	c.msgBuffer.WriteString("&type=")
	switch orderType {
	case common.OrderTypeLimit:
		if isLimitMaker {
			c.msgBuffer.WriteString("LIMIT_MAKER")
		} else {
			c.msgBuffer.WriteString("LIMIT")
		}
	case common.OrderTypeMarket:
		c.msgBuffer.WriteString("MARKET")
	}

	signature := c.signEd25519(c.msgBuffer.Bytes())

	// Build WebSocket request message
	requestID := c.requestID.Add(1)
	c.pendingOrderRequests.Store(requestID, clientOrderID)
	msg := c.buildOrderNewRequest(symbol.Name, side, orderType, timeInForce, price, quantity, clientOrderID, timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// CancelOrder cancels an order by orderID via WebSocket
func (c *BinanceSpotExecutionClient) CancelOrder(symbolID int, orderID int, clientOrderID int) error {
	symbol, err := c.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing (sorted alphabetically per Binance spec)
	c.msgBuffer.Reset()
	timestamp := time.Now().UnixMilli()

	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString("&orderId=")
	c.msgBuffer.WriteString(strconv.Itoa(orderID))
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&symbol=")
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	c.pendingCancelRequests.Store(requestID, clientOrderID)
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

	// Build params for signing (sorted alphabetically per Binance spec)
	c.msgBuffer.Reset()
	timestamp := time.Now().UnixMilli()

	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&symbol=")
	c.msgBuffer.WriteString(symbol.Name)
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildCancelAllOrdersRequest(symbol.Name, timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// ReqBalanceSnapshot requests account balance snapshot via WebSocket.
// walletType is accepted for interface compatibility; Binance spot always queries the spot account.
func (c *BinanceSpotExecutionClient) ReqBalanceSnapshot(walletType common.WalletType) error {
	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Build params for signing (same order as JSON params, including apiKey but not signature)
	c.msgBuffer.Reset()

	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString("&recvWindow=")
	c.msgBuffer.WriteString(strconv.Itoa(wsAPIRecvWindow))
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildAccountStatusRequest(timestamp, signature, requestID)

	return c.sendMessage(msg)
}

// SubscribeOrderUpdate subscribes to order status update events
// Binance: Subscribes to user data stream if not already subscribed, enables order update filtering
func (c *BinanceSpotExecutionClient) SubscribeOrderUpdate() error {
	c.subOrderUpdate.Store(true)
	// If already connected, subscribe now; otherwise will subscribe on Connect()
	if c.connected.Load() {
		return c.ensureUserDataStreamSubscribed()
	}
	return nil
}

// SubscribeFill subscribes to execution/fill events
// Binance: Subscribes to user data stream if not already subscribed, enables fill filtering
func (c *BinanceSpotExecutionClient) SubscribeFill() error {
	c.subFill.Store(true)
	// If already connected, subscribe now; otherwise will subscribe on Connect()
	if c.connected.Load() {
		return c.ensureUserDataStreamSubscribed()
	}
	return nil
}

// SubscribeBalance subscribes to wallet/balance update events
// Binance: Subscribes to user data stream if not already subscribed, enables balance filtering
func (c *BinanceSpotExecutionClient) SubscribeBalance() error {
	c.subBalance.Store(true)
	// If already connected, subscribe now; otherwise will subscribe on Connect()
	if c.connected.Load() {
		return c.ensureUserDataStreamSubscribed()
	}
	return nil
}

// hasAnySubscription returns true if any subscription flag is set
func (c *BinanceSpotExecutionClient) hasAnySubscription() bool {
	return c.subOrderUpdate.Load() || c.subFill.Load() || c.subBalance.Load()
}

// ensureUserDataStreamSubscribed subscribes to user data stream if not already subscribed
// Uses userDataStream.subscribe.signature method which requires apiKey, timestamp, signature
func (c *BinanceSpotExecutionClient) ensureUserDataStreamSubscribed() error {
	// Check if already subscribed
	if c.userDataStreamSubscribed.Load() {
		return nil
	}

	c.bufLock.Lock()
	defer c.bufLock.Unlock()

	// Double-check after acquiring lock
	if c.userDataStreamSubscribed.Load() {
		return nil
	}

	// Build params for signing (only apiKey and timestamp)
	c.msgBuffer.Reset()

	timestamp := time.Now().UnixMilli()
	c.msgBuffer.WriteString("apiKey=")
	c.msgBuffer.WriteString(c.apiKey.Key)
	c.msgBuffer.WriteString("&timestamp=")
	c.msgBuffer.WriteString(strconv.FormatInt(timestamp, 10))

	signature := c.signEd25519(c.msgBuffer.Bytes())

	requestID := c.requestID.Add(1)
	msg := c.buildUserDataStreamSubscribeRequest(timestamp, signature, requestID)

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	c.userDataStreamSubscribed.Store(true)
	return nil
}

// buildUserDataStreamSubscribeRequest builds a JSON message for userDataStream.subscribe.signature
// This method is used for unauthenticated sessions (no session.logon)
// Required params: apiKey, timestamp, signature
func (c *BinanceSpotExecutionClient) buildUserDataStreamSubscribeRequest(timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(256)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"userDataStream.subscribe.signature","params":{`)

	buf.WriteString(`"apiKey":"`)
	buf.WriteString(c.apiKey.Key)
	buf.WriteString(`"`)

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"signature":"`)
	buf.WriteString(signature)
	buf.WriteString(`"`)

	buf.WriteString(`}}`)

	return buf.Bytes()
}

// buildAccountStatusRequest builds a JSON message for account.status
func (c *BinanceSpotExecutionClient) buildAccountStatusRequest(timestamp int64, signature string, requestID uint64) []byte {
	var buf bytes.Buffer
	buf.Grow(256)

	buf.WriteString(`{"id":"`)
	buf.WriteString(strconv.FormatUint(requestID, 10))
	buf.WriteString(`","method":"account.status","params":{`)

	buf.WriteString(`"apiKey":"`)
	buf.WriteString(c.apiKey.Key)
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
func (c *BinanceSpotExecutionClient) buildOrderNewRequest(symbol string, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64, clientOrderID int, timestamp int64, signature string, requestID uint64) []byte {
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

	buf.WriteString(`,"newClientOrderId":"`)
	buf.WriteString(strconv.Itoa(clientOrderID))
	buf.WriteString(`"`)

	buf.WriteString(`,"timestamp":`)
	buf.WriteString(strconv.FormatInt(timestamp, 10))

	buf.WriteString(`,"recvWindow":`)
	buf.WriteString(strconv.Itoa(wsAPIRecvWindow))

	buf.WriteString(`,"apiKey":"`)
	buf.WriteString(c.apiKey.Key)
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
	buf.WriteString(c.apiKey.Key)
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
	buf.WriteString(c.apiKey.Key)
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
	// Check for error (e.g. -2010 "Order would immediately match and take.")
	errCode, err := jsonparser.GetInt(data, "error", "code")
	if err == nil && errCode != 0 {
		errMsg, _ := jsonparser.GetString(data, "error", "msg")
		log().Error().Int64("code", errCode).Str("msg", errMsg).Msg("WebSocket API error")
		// Publish OrderRejected when this is an order.place error (we have request id -> clientOrderID).
		// Binance echoes "id" as string or int; parse both.
		reqID := parseRequestID(data)
		if reqID != 0 {
			if v, ok := c.pendingOrderRequests.LoadAndDelete(reqID); ok {
				clientOrderID := v.(int)
				updatedAt := uint64(time.Now().UnixNano())
				c.publishOrderRejected(clientOrderID, -1, errMsg, updatedAt)
			} else if v, ok := c.pendingCancelRequests.LoadAndDelete(reqID); ok {
				clientOrderID := v.(int)
				updatedAt := uint64(time.Now().UnixNano())
				c.publishOrderCanceled(clientOrderID, -1, int(errCode), updatedAt)
			}
		}
		return
	}

	// Check for user data stream event (has "event" field with "e" inside)
	eventData, _, _, err := jsonparser.Get(data, "event")
	if err == nil && len(eventData) > 0 {
		c.processUserDataStreamEvent(eventData)
		return
	}

	// Check for result (request/response pattern)
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
		if reqID := parseRequestID(data); reqID != 0 {
			c.pendingOrderRequests.Delete(reqID)
			c.pendingCancelRequests.Delete(reqID)
		}
	}
}

// processUserDataStreamEvent processes user data stream events
// Event types:
// - outboundAccountPosition: account balance updates -> TopicEventBalanceUpdate (if subscribed)
// - executionReport: order execution updates -> TopicEventOrderUpdate (if subscribed)
// - balanceUpdate: delta balance updates -> unhandled (warning logged)
// - listStatus: OCO order updates -> unhandled (warning logged)
// - Others -> unknown (error logged)
//
// Events are only published if the corresponding subscription is active.
// Unsubscribed events are logged as unhandled.
func (c *BinanceSpotExecutionClient) processUserDataStreamEvent(data []byte) {
	eventType, err := jsonparser.GetString(data, "e")
	if err != nil {
		log().Error().Err(err).Msg("Failed to get event type from user data stream event")
		return
	}

	switch eventType {
	case "outboundAccountPosition":
		if c.subBalance.Load() {
			c.processOutboundAccountPosition(data)
		} else {
			log().Debug().
				Int("accountID", c.accountID).
				Str("eventType", eventType).
				Msg("Received balance event but not subscribed - ignoring")
		}
	case "executionReport":
		// executionReport contains both order updates and fills
		// Process based on which subscriptions are active
		c.processExecutionReport(data)
	case "balanceUpdate":
		log().Debug().
			Int("accountID", c.accountID).
			Str("eventType", eventType).
			Msg("Received balanceUpdate event (delta update) - not handled")
	case "listStatus":
		log().Debug().
			Int("accountID", c.accountID).
			Str("eventType", eventType).
			Msg("Received listStatus event (OCO order) - not handled")
	default:
		log().Warn().
			Int("accountID", c.accountID).
			Str("eventType", eventType).
			Msg("Received unknown user data stream event type")
	}
}

// processOutboundAccountPosition processes outboundAccountPosition events
// and publishes BalanceUpdate to the event bus
// Format:
//
//	{
//	  "e": "outboundAccountPosition",
//	  "E": 1564034571105,  // Event Time
//	  "u": 1564034571073,  // Time of last account update
//	  "B": [{"a": "ETH", "f": "10000.000000", "l": "0.000000"}, ...]
//	}
func (c *BinanceSpotExecutionClient) processOutboundAccountPosition(data []byte) {
	// Parse update time
	updateTime, _ := jsonparser.GetInt(data, "u")

	// Count non-zero balances first
	balanceCount := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		freeStr, _ := jsonparser.GetString(value, "f")
		lockedStr, _ := jsonparser.GetString(value, "l")
		free := parseFloat64([]byte(freeStr))
		locked := parseFloat64([]byte(lockedStr))
		if free > 0 || locked > 0 {
			balanceCount++
		}
	}, "B")

	if balanceCount == 0 {
		log().Debug().Int("accountID", c.accountID).Msg("No non-zero balances in outboundAccountPosition")
		return
	}

	// Allocate balances slice
	balances := make([]common.Balance, 0, balanceCount)

	// Parse balances
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		asset, _ := jsonparser.GetString(value, "a")
		freeStr, _ := jsonparser.GetString(value, "f")
		lockedStr, _ := jsonparser.GetString(value, "l")

		free := parseFloat64([]byte(freeStr))
		locked := parseFloat64([]byte(lockedStr))
		total := free + locked

		// Only include non-zero balances
		if total > 0 {
			balances = append(balances, common.Balance{
				TokenID:   c.getTokenID(asset),
				Available: free,
				Locked:    locked,
				Total:     total,
			})
		}
	}, "B")

	// Create and publish BalanceUpdate event
	balanceUpdate := event.BalanceUpdate{
		AccountID: c.accountID,
		WalletID:  c.walletID,
		Balances:  balances,
		UpdatedAt: uint64(updateTime) * 1_000_000, // Convert ms to ns
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
		Int("walletID", c.walletID).
		Int("balanceCount", len(balances)).
		Msg("Published balance update from user data stream")
}

// processExecutionReport processes executionReport events from user data stream
// and publishes appropriate order events and Fill events to the event bus
// Only publishes events if the corresponding subscription is active.
// Format:
//
//	{
//	  "e": "executionReport",
//	  "E": 1499405658658,  // Event time
//	  "s": "ETHBTC",       // Symbol
//	  "c": "clientOrdId",  // Client order ID
//	  "S": "BUY",          // Side
//	  "o": "LIMIT",        // Order type
//	  "f": "GTC",          // Time in force
//	  "q": "1.00000000",   // Order quantity
//	  "p": "0.10264410",   // Order price
//	  "x": "NEW",          // Current execution type
//	  "X": "NEW",          // Current order status
//	  "r": "NONE",         // Order reject reason
//	  "i": 4293153,        // Order ID
//	  "l": "0.00000000",   // Last executed quantity
//	  "z": "0.00000000",   // Cumulative filled quantity
//	  "L": "0.00000000",   // Last executed price
//	  "n": "0",            // Commission amount
//	  "N": null,           // Commission asset
//	  "T": 1499405658657,  // Transaction time
//	  "t": -1,             // Trade ID
//	}
func (c *BinanceSpotExecutionClient) processExecutionReport(data []byte) {
	// Check if any relevant subscription is active
	subOrderUpdate := c.subOrderUpdate.Load()
	subFill := c.subFill.Load()

	if !subOrderUpdate && !subFill {
		log().Debug().
			Int("accountID", c.accountID).
			Msg("Received executionReport but neither order update nor fill subscribed - ignoring")
		return
	}

	// Parse common order fields
	orderID, _ := jsonparser.GetInt(data, "i")
	clientOrderIDStr, _ := jsonparser.GetString(data, "c")
	clientOrderID, _ := strconv.Atoi(clientOrderIDStr)
	if clientOrderID == 0 {
		origStr, _ := jsonparser.GetString(data, "C")
		clientOrderID, _ = strconv.Atoi(origStr)
	}
	status, _ := jsonparser.GetString(data, "X")
	transactTime, _ := jsonparser.GetInt(data, "T")
	updatedAt := uint64(transactTime) * 1_000_000 // Convert ms to ns

	// Publish appropriate order event based on status (if subscribed)
	switch strings.ToUpper(status) {
	case "NEW":
		if subOrderUpdate {
			c.publishOrderAccepted(clientOrderID, int(orderID), updatedAt)
		}

	case "PARTIALLY_FILLED":
		if subOrderUpdate {
			executedQtyStr, _ := jsonparser.GetString(data, "z")
			executedQty := parseFloat64([]byte(executedQtyStr))
			c.publishOrderPartiallyFilled(clientOrderID, int(orderID), executedQty, updatedAt)
		}
		// Also publish Fill event (if subscribed)
		if subFill {
			c.publishFillFromExecutionReport(data, clientOrderID, int(orderID), transactTime)
		}

	case "FILLED":
		if subOrderUpdate {
			executedQtyStr, _ := jsonparser.GetString(data, "z")
			executedQty := parseFloat64([]byte(executedQtyStr))
			c.publishOrderFilled(clientOrderID, int(orderID), executedQty, updatedAt)
		}
		// Also publish Fill event (if subscribed)
		if subFill {
			c.publishFillFromExecutionReport(data, clientOrderID, int(orderID), transactTime)
		}

	case "CANCELED", "EXPIRED":
		if subOrderUpdate {
			c.publishOrderCanceled(clientOrderID, int(orderID), 0, updatedAt)
		}

	case "REJECTED":
		if subOrderUpdate {
			rejectReason, _ := jsonparser.GetString(data, "r")
			c.publishOrderRejected(clientOrderID, int(orderID), rejectReason, updatedAt)
		}

	default:
		if subOrderUpdate {
			c.publishOrderUnknownStatus(clientOrderID, int(orderID), status)
		}
	}

	log().Debug().
		Int("accountID", c.accountID).
		Int("orderID", int(orderID)).
		Str("status", status).
		Bool("subOrderUpdate", subOrderUpdate).
		Bool("subFill", subFill).
		Msg("Processed execution report")
}

// publishFillFromExecutionReport extracts fill data from execution report and publishes Fill event
func (c *BinanceSpotExecutionClient) publishFillFromExecutionReport(data []byte, clientOrderID int, orderID int, transactTime int64) {
	lastQtyStr, _ := jsonparser.GetString(data, "l")
	lastQty := parseFloat64([]byte(lastQtyStr))

	if lastQty <= 0 {
		return // No fill in this report
	}

	lastPriceStr, _ := jsonparser.GetString(data, "L")
	lastPrice := parseFloat64([]byte(lastPriceStr))

	commissionStr, _ := jsonparser.GetString(data, "n")
	commission := parseFloat64([]byte(commissionStr))

	commissionAsset, _ := jsonparser.GetString(data, "N")
	tradeID, _ := jsonparser.GetInt(data, "t")
	symbolStr, _ := jsonparser.GetString(data, "s")
	sideStr, _ := jsonparser.GetString(data, "S")
	symbolID := 0
	if c.catalog != nil {
		if sym, err := c.catalog.GetSymbolByExchangeAndName(c.account.Exchange, symbolStr); err == nil {
			symbolID = sym.ID
		}
	}
	side := parseSide(sideStr)
	isMaker, _ := jsonparser.GetBoolean(data, "m")

	fill := event.Execution{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		SymbolID:      symbolID,
		Side:          side,
		IsMaker:       isMaker,
		FillID:        int(tradeID),
		FilledQty:     lastQty,
		FilledPrice:   lastPrice,
		FeeCcyID:      c.getTokenID(commissionAsset),
		FeeQty:        commission,
		FilledAt:      uint64(transactTime) * 1_000_000,
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
		Float64("lastQty", lastQty).
		Float64("lastPrice", lastPrice).
		Msg("Published fill from execution report")
}

// publishOrderAccepted publishes an OrderAccepted event
func (c *BinanceSpotExecutionClient) publishOrderAccepted(clientOrderID, orderID int, createdAt uint64) {
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
func (c *BinanceSpotExecutionClient) publishOrderPartiallyFilled(clientOrderID, orderID int, executedQty float64, updatedAt uint64) {
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
func (c *BinanceSpotExecutionClient) publishOrderFilled(clientOrderID, orderID int, executedQty float64, updatedAt uint64) {
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
func (c *BinanceSpotExecutionClient) publishOrderCanceled(clientOrderID, orderID int, errorCode int, updatedAt uint64) {
	e := event.OrderCanceled{
		ClientOrderID: clientOrderID,
		OrderID:       orderID,
		AccountID:     c.accountID,
		ErrorCode:     errorCode,
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
func (c *BinanceSpotExecutionClient) publishOrderRejected(clientOrderID, orderID int, reason string, updatedAt uint64) {
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
func (c *BinanceSpotExecutionClient) publishOrderUnknownStatus(clientOrderID, orderID int, status string) {
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

// processOrderResponse processes order-related responses from request/response pattern.
// When the user data stream is active, order lifecycle events (accepted, filled, etc.)
// and executions arrive via executionReport, so this method skips publishing them to
// avoid duplication. It only publishes as a fallback when the stream is not subscribed.
func (c *BinanceSpotExecutionClient) processOrderResponse(data []byte) {
	if c.userDataStreamSubscribed.Load() {
		return
	}

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
	updatedAt := uint64(updateTime) * 1_000_000 // Convert ms to ns

	// Publish appropriate order event based on status
	switch strings.ToUpper(status) {
	case "NEW":
		c.publishOrderAccepted(clientOrderID, int(orderIDInt), updatedAt)

	case "PARTIALLY_FILLED":
		c.publishOrderPartiallyFilled(clientOrderID, int(orderIDInt), executedQty, updatedAt)
		if _, _, _, fillErr := jsonparser.Get(data, "fills"); fillErr == nil {
			c.processFills(data, clientOrderID, int(orderIDInt))
		}

	case "FILLED":
		c.publishOrderFilled(clientOrderID, int(orderIDInt), executedQty, updatedAt)
		if _, _, _, fillErr := jsonparser.Get(data, "fills"); fillErr == nil {
			c.processFills(data, clientOrderID, int(orderIDInt))
		}

	case "CANCELED", "EXPIRED":
		c.publishOrderCanceled(clientOrderID, int(orderIDInt), 0, updatedAt)

	case "REJECTED":
		c.publishOrderRejected(clientOrderID, int(orderIDInt), "Order rejected", updatedAt)

	default:
		c.publishOrderUnknownStatus(clientOrderID, int(orderIDInt), status)
	}
}

// processFills processes fill information from order responses
func (c *BinanceSpotExecutionClient) processFills(data []byte, clientOrderID int, orderID int) {
	symbolStr, _ := jsonparser.GetString(data, "symbol")
	sideStr, _ := jsonparser.GetString(data, "side")
	symbolID := 0
	if c.catalog != nil {
		if sym, err := c.catalog.GetSymbolByExchangeAndName(c.account.Exchange, symbolStr); err == nil {
			symbolID = sym.ID
		}
	}
	side := parseSide(sideStr)

	fillIdx := 0
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		priceStr, _ := jsonparser.GetString(value, "price")
		qtyStr, _ := jsonparser.GetString(value, "qty")
		commissionStr, _ := jsonparser.GetString(value, "commission")
		commissionAsset, _ := jsonparser.GetString(value, "commissionAsset")
		tradeID, _ := jsonparser.GetInt(value, "tradeId")
		isMaker, _ := jsonparser.GetBoolean(value, "maker")

		fill := event.Execution{
			ClientOrderID: clientOrderID,
			OrderID:       orderID,
			AccountID:     c.accountID,
			SymbolID:      symbolID,
			Side:          side,
			IsMaker:       isMaker,
			FillID:        int(tradeID),
			FilledQty:     parseFloat64([]byte(qtyStr)),
			FilledPrice:   parseFloat64([]byte(priceStr)),
			FeeCcyID:      c.getTokenID(commissionAsset),
			FeeQty:        parseFloat64([]byte(commissionStr)),
			FilledAt:      uint64(time.Now().UnixNano()),
		}

		// Publish fill event
		size := uint64(fill.GetBufferLength())
		fillOffset, buf := c.msgBus.Allocate(size)
		fill.Encode(buf)
		c.msgBus.Publish(msgbus.EventRef{
			Topic:  event.TopicEventExecution,
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

	// Calculate size and allocate directly from event bus
	// Layout: [AccountID(8)][WalletID(8)][BalancesLen(4)][Padding(4)][Balance1][Balance2]...[BalanceN]
	size := uint64(event.RespBalanceSnapshotHeaderSize) + uint64(balanceCount)*uint64(event.BalanceSize)
	offset, buf := c.msgBus.Allocate(size)

	// Write header directly to buffer
	pos := 0
	// AccountID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(c.accountID))
	pos += 8
	// WalletID (8 bytes)
	binary.LittleEndian.PutUint64(buf[pos:], uint64(c.walletID))
	pos += 8
	// BalancesLen (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], uint32(balanceCount))
	pos += 4
	// Padding (4 bytes)
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4

	// Write balances directly to buffer during parsing
	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		asset, _ := jsonparser.GetString(value, "asset")
		freeStr, _ := jsonparser.GetString(value, "free")
		lockedStr, _ := jsonparser.GetString(value, "locked")

		free := parseFloat64([]byte(freeStr))
		locked := parseFloat64([]byte(lockedStr))
		total := free + locked

		// Only include non-zero balances
		if total > 0 {
			// Write Balance directly to buffer using unsafe pointer
			balance := (*common.Balance)(unsafe.Pointer(&buf[pos]))
			balance.TokenID = c.getTokenID(asset)
			balance.Available = free
			balance.Locked = locked
			balance.Total = total
			pos += event.BalanceSize
		}
	}, "balances")

	c.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventRespBalanceSnapshot,
		Index:  offset,
		Length: size,
	})

	log().Debug().Int("accountID", c.accountID).Int("walletID", c.walletID).Int("balanceCount", balanceCount).Msg("Published balance snapshot")
}

// parseOrderStatus converts Binance order status string to common.OrderStatus
// getTokenID gets the token ID from the catalog by asset name
func (c *BinanceSpotExecutionClient) getTokenID(asset string) int {
	if c.catalog != nil {
		if tokenID, err := c.catalog.GetTokenIDByName(asset); err == nil {
			return tokenID
		}
	}
	return 0
}

// parseRequestID extracts the WebSocket response "id" from data.
// Binance echoes id as string or int; parse both so we can match pending order.place requests.
func parseRequestID(data []byte) uint64 {
	if id, err := jsonparser.GetInt(data, "id"); err == nil {
		return uint64(id)
	}
	if s, err := jsonparser.GetString(data, "id"); err == nil {
		id, _ := strconv.ParseUint(s, 10, 64)
		return id
	}
	return 0
}

// parseSide maps Binance side string ("BUY"/"SELL") to common.Side.
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
