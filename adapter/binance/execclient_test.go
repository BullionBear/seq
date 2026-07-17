package binance

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// Unit Tests for Ed25519 Signing
// ============================================================================

func TestSignEd25519(t *testing.T) {
	// Generate a test Ed25519 key pair
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key: %v", err)
	}

	// Create a mock client with the private key
	client := &BinanceSpotExecutionClient{
		privateKey: privateKey,
	}

	payload := []byte("timestamp=1234567890&recvWindow=5000")
	signature := client.signEd25519(payload)

	// Verify signature is base64 encoded
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Errorf("Signature is not valid base64: %v", err)
	}

	// Verify signature length (Ed25519 signatures are 64 bytes)
	if len(sigBytes) != ed25519.SignatureSize {
		t.Errorf("Expected signature size %d, got %d", ed25519.SignatureSize, len(sigBytes))
	}

	// Verify the signature is valid
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if !ed25519.Verify(publicKey, payload, sigBytes) {
		t.Error("Signature verification failed")
	}
}

// ============================================================================
// Unit Tests for Request Building
// ============================================================================

func TestBuildAccountStatusRequest(t *testing.T) {
	// Generate a test Ed25519 key pair
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key: %v", err)
	}

	client := &BinanceSpotExecutionClient{
		privateKey: privateKey,
		apiKey: catalog.APIKey{
			Key: "test-api-key",
		},
	}

	timestamp := int64(1234567890000)
	signature := "test-signature"
	requestID := uint64(1)

	msg := client.buildAccountStatusRequest(timestamp, signature, requestID)

	// Verify the message contains expected fields
	msgStr := string(msg)
	if !contains(msgStr, `"method":"account.status"`) {
		t.Error("Missing method field")
	}
	if !contains(msgStr, `"id":"1"`) {
		t.Error("Missing or incorrect id field")
	}
	if !contains(msgStr, `"apiKey":"test-api-key"`) {
		t.Error("Missing apiKey field")
	}
	if !contains(msgStr, `"timestamp":1234567890000`) {
		t.Error("Missing timestamp field")
	}
	if !contains(msgStr, `"signature":"test-signature"`) {
		t.Error("Missing signature field")
	}
}

func TestBuildOrderNewRequest(t *testing.T) {
	client := &BinanceSpotExecutionClient{
		apiKey: catalog.APIKey{
			Key: "test-api-key",
		},
	}

	tests := []struct {
		name        string
		symbol      string
		side        common.Side
		orderType   common.OrderType
		timeInForce common.TimeInForce
		price       float64
		quantity    float64
		wantType    string
		wantTIF     bool
	}{
		{
			name:        "limit GTC order",
			symbol:      "BTCUSDT",
			side:        common.SideBuy,
			orderType:   common.OrderTypeLimit,
			timeInForce: common.TimeInForceGTC,
			price:       50000.0,
			quantity:    0.001,
			wantType:    "LIMIT",
			wantTIF:     true,
		},
		{
			name:        "limit maker order",
			symbol:      "BTCUSDT",
			side:        common.SideSell,
			orderType:   common.OrderTypeLimit,
			timeInForce: common.TimeInForcePO,
			price:       50000.0,
			quantity:    0.001,
			wantType:    "LIMIT_MAKER",
			wantTIF:     false,
		},
		{
			name:        "market order",
			symbol:      "ETHUSDT",
			side:        common.SideBuy,
			orderType:   common.OrderTypeMarket,
			timeInForce: common.TimeInForceGTC,
			price:       0,
			quantity:    0.1,
			wantType:    "MARKET",
			wantTIF:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := client.buildOrderNewRequest(
				tt.symbol, tt.side, tt.orderType, tt.timeInForce,
				tt.price, tt.quantity, 12345, 1234567890000, "test-sig", 1,
			)
			msgStr := string(msg)

			if !contains(msgStr, `"method":"order.place"`) {
				t.Error("Missing method field")
			}
			if !contains(msgStr, `"symbol":"`+tt.symbol+`"`) {
				t.Error("Missing or incorrect symbol field")
			}
			if !contains(msgStr, `"type":"`+tt.wantType+`"`) {
				t.Errorf("Expected type %s", tt.wantType)
			}
			if tt.wantTIF && !contains(msgStr, `"timeInForce":`) {
				t.Error("Missing timeInForce field for limit order")
			}
			if !tt.wantTIF && contains(msgStr, `"timeInForce":`) {
				t.Error("Unexpected timeInForce field")
			}
		})
	}
}

// ============================================================================
// Unit Tests for Response Processing
// ============================================================================

func TestProcessAccountStatusResponse(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus:    eb,
		accountID: 123,
	}

	// Mock Binance account.status response
	accountResponse := []byte(`{
		"makerCommission": 15,
		"takerCommission": 15,
		"buyerCommission": 0,
		"sellerCommission": 0,
		"canTrade": true,
		"canWithdraw": true,
		"canDeposit": true,
		"updateTime": 1672531200000,
		"accountType": "SPOT",
		"balances": [
			{"asset": "BTC", "free": "1.50000000", "locked": "0.50000000"},
			{"asset": "ETH", "free": "10.00000000", "locked": "0.00000000"},
			{"asset": "USDT", "free": "0.00000000", "locked": "0.00000000"}
		]
	}`)

	client.processAccountStatusResponse(accountResponse)

	// Verify event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected balance snapshot event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventRespBalanceSnapshot {
		t.Errorf("Expected TopicEventReqBalanceSnapshot, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	snapshot, err := event.NewRespBalanceSnapshotFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if snapshot.AccountID != 123 {
		t.Errorf("Expected AccountID 123, got %d", snapshot.AccountID)
	}

	// Should only have 2 non-zero balances (BTC and ETH)
	if len(snapshot.Balances) != 2 {
		t.Fatalf("Expected 2 balances, got %d", len(snapshot.Balances))
	}

	// Check BTC balance
	btc := snapshot.Balances[0]
	if btc.Available != 1.5 || btc.Locked != 0.5 || btc.Total != 2.0 {
		t.Errorf("BTC balance mismatch: got Available=%f, Locked=%f, Total=%f",
			btc.Available, btc.Locked, btc.Total)
	}

	// Check ETH balance
	eth := snapshot.Balances[1]
	if eth.Available != 10.0 || eth.Locked != 0.0 || eth.Total != 10.0 {
		t.Errorf("ETH balance mismatch: got Available=%f, Locked=%f, Total=%f",
			eth.Available, eth.Locked, eth.Total)
	}
}

func TestProcessOrderResponse(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus: eb,
	}

	// Mock Binance order response for NEW order
	orderResponse := []byte(`{
		"symbol": "BTCUSDT",
		"orderId": 12345678,
		"orderListId": -1,
		"clientOrderId": "123",
		"transactTime": 1672531200000,
		"price": "50000.00000000",
		"origQty": "0.00100000",
		"executedQty": "0.00000000",
		"cummulativeQuoteQty": "0.00000000",
		"status": "NEW",
		"timeInForce": "GTC",
		"type": "LIMIT",
		"side": "BUY"
	}`)

	client.processOrderResponse(orderResponse)

	// Verify event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected OrderAccepted event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventOrderAccepted {
		t.Errorf("Expected TopicEventOrderAccepted, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	orderAccepted, err := event.NewOrderAcceptedFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if orderAccepted.OrderID != 12345678 {
		t.Errorf("Expected OrderID 12345678, got %d", orderAccepted.OrderID)
	}

	if orderAccepted.ClientOrderID != 123 {
		t.Errorf("Expected ClientOrderID 123, got %d", orderAccepted.ClientOrderID)
	}
}

func TestProcessOrderResponseCanceled(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus: eb,
	}

	// Mock Binance order response for CANCELED order
	orderResponse := []byte(`{
		"symbol": "BTCUSDT",
		"orderId": 12345678,
		"orderListId": -1,
		"clientOrderId": "123",
		"transactTime": 1672531200000,
		"price": "50000.00000000",
		"origQty": "0.00100000",
		"executedQty": "0.00000000",
		"status": "CANCELED"
	}`)

	client.processOrderResponse(orderResponse)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected OrderCanceled event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventOrderCanceled {
		t.Errorf("Expected TopicEventOrderCanceled, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	orderCanceled, err := event.NewOrderCanceledFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if orderCanceled.OrderID != 12345678 {
		t.Errorf("Expected OrderID 12345678, got %d", orderCanceled.OrderID)
	}
}

// ============================================================================
// Unit Tests for User Data Stream Processing
// ============================================================================

func TestProcessOutboundAccountPosition(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus:    eb,
		accountID: 123,
	}

	// Mock outboundAccountPosition event
	eventData := []byte(`{
		"e": "outboundAccountPosition",
		"E": 1564034571105,
		"u": 1564034571073,
		"B": [
			{"a": "ETH", "f": "10000.000000", "l": "500.000000"},
			{"a": "BTC", "f": "1.50000000", "l": "0.00000000"},
			{"a": "USDT", "f": "0.00000000", "l": "0.00000000"}
		]
	}`)

	client.processOutboundAccountPosition(eventData)

	// Verify event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected balance update event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventBalanceUpdate {
		t.Errorf("Expected TopicEventBalanceUpdate, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	balanceUpdate, err := event.NewBalanceUpdateFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if balanceUpdate.AccountID != 123 {
		t.Errorf("Expected AccountID 123, got %d", balanceUpdate.AccountID)
	}

	// Should only have 2 non-zero balances (ETH and BTC)
	if len(balanceUpdate.Balances) != 2 {
		t.Fatalf("Expected 2 balances, got %d", len(balanceUpdate.Balances))
	}

	// Check ETH balance
	eth := balanceUpdate.Balances[0]
	if eth.Available != 10000.0 || eth.Locked != 500.0 || eth.Total != 10500.0 {
		t.Errorf("ETH balance mismatch: got Available=%f, Locked=%f, Total=%f",
			eth.Available, eth.Locked, eth.Total)
	}

	// Check BTC balance
	btc := balanceUpdate.Balances[1]
	if btc.Available != 1.5 || btc.Locked != 0.0 || btc.Total != 1.5 {
		t.Errorf("BTC balance mismatch: got Available=%f, Locked=%f, Total=%f",
			btc.Available, btc.Locked, btc.Total)
	}
}

func TestProcessExecutionReport(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus:    eb,
		accountID: 123,
	}

	// Mock executionReport event for a new order
	eventData := []byte(`{
		"e": "executionReport",
		"E": 1499405658658,
		"s": "ETHBTC",
		"c": "456",
		"S": "BUY",
		"o": "LIMIT",
		"f": "GTC",
		"q": "1.00000000",
		"p": "0.10264410",
		"x": "NEW",
		"X": "NEW",
		"i": 4293153,
		"l": "0.00000000",
		"z": "0.00000000",
		"L": "0.00000000",
		"n": "0",
		"N": null,
		"T": 1499405658657,
		"t": -1
	}`)

	client.SubscribeOrderUpdate()
	client.processExecutionReport(eventData)

	// Verify OrderAccepted event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected OrderAccepted event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventOrderAccepted {
		t.Errorf("Expected TopicEventOrderAccepted, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	orderAccepted, err := event.NewOrderAcceptedFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if orderAccepted.OrderID != 4293153 {
		t.Errorf("Expected OrderID 4293153, got %d", orderAccepted.OrderID)
	}

	if orderAccepted.ClientOrderID != 456 {
		t.Errorf("Expected ClientOrderID 456, got %d", orderAccepted.ClientOrderID)
	}
}

func TestProcessExecutionReportWithFill(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus:    eb,
		accountID: 123,
	}

	// Mock executionReport event with a partial fill
	eventData := []byte(`{
		"e": "executionReport",
		"E": 1499405658658,
		"s": "ETHBTC",
		"c": "789",
		"S": "BUY",
		"o": "LIMIT",
		"f": "GTC",
		"q": "1.00000000",
		"p": "0.10264410",
		"x": "TRADE",
		"X": "PARTIALLY_FILLED",
		"i": 4293154,
		"l": "0.50000000",
		"z": "0.50000000",
		"L": "0.10264410",
		"n": "0.00001000",
		"N": "ETH",
		"T": 1499405658657,
		"t": 12345
	}`)

	client.SubscribeOrderUpdate()
	client.SubscribeFill()
	client.processExecutionReport(eventData)

	// Should receive OrderPartiallyFilled first, then Fill
	var partialFillEvent msgbus.Event
	var fillEvent msgbus.Event
	eventCount := 0

	for i := 0; i < 2; i++ {
		ok := eb.Poll(func(e msgbus.Event) {
			if e.Ref.Topic == event.TopicEventOrderPartialFill {
				partialFillEvent = e
				eventCount++
			} else if e.Ref.Topic == event.TopicEventExecution {
				fillEvent = e
				eventCount++
			}
		})
		if !ok {
			break
		}
	}

	if eventCount != 2 {
		t.Fatalf("Expected 2 events (partial fill + fill), got %d", eventCount)
	}

	// Verify OrderPartiallyFilled
	orderBuf := eb.ReadBuffer(partialFillEvent.Ref.Index, partialFillEvent.Ref.Length)
	orderPartiallyFilled, err := event.NewOrderPartiallyFilledFromBytes(orderBuf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if orderPartiallyFilled.OrderID != 4293154 {
		t.Errorf("Expected OrderID 4293154, got %d", orderPartiallyFilled.OrderID)
	}

	if orderPartiallyFilled.ExecutedQty != 0.5 {
		t.Errorf("Expected ExecutedQty 0.5, got %f", orderPartiallyFilled.ExecutedQty)
	}

	// Verify Execution
	fillBuf := eb.ReadBuffer(fillEvent.Ref.Index, fillEvent.Ref.Length)
	fill, err := event.NewExecutionFromBytes(fillBuf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if fill.OrderID != 4293154 {
		t.Errorf("Expected OrderID 4293154, got %d", fill.OrderID)
	}

	if fill.FillID != 12345 {
		t.Errorf("Expected FillID 12345, got %d", fill.FillID)
	}

	if fill.FilledQty != 0.5 {
		t.Errorf("Expected FilledQty 0.5, got %f", fill.FilledQty)
	}

	if math.Abs(fill.FilledPrice-0.10264410) > 1e-10 {
		t.Errorf("Expected FilledPrice 0.10264410, got %f", fill.FilledPrice)
	}

	if math.Abs(fill.FeeQty-0.00001) > 1e-10 {
		t.Errorf("Expected FeeQty 0.00001, got %f", fill.FeeQty)
	}
}

func TestProcessUserDataStreamEvent(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := &BinanceSpotExecutionClient{
		msgBus:    eb,
		accountID: 123,
	}
	client.SubscribeBalance()
	client.SubscribeOrderUpdate()

	tests := []struct {
		name          string
		eventData     []byte
		expectEvent   bool
		expectedTopic event.Topic
	}{
		{
			name: "outboundAccountPosition",
			eventData: []byte(`{
				"e": "outboundAccountPosition",
				"E": 1564034571105,
				"u": 1564034571073,
				"B": [{"a": "BTC", "f": "1.0", "l": "0.0"}]
			}`),
			expectEvent:   true,
			expectedTopic: event.TopicEventBalanceUpdate,
		},
		{
			name: "executionReport NEW",
			eventData: []byte(`{
				"e": "executionReport",
				"E": 1499405658658,
				"s": "ETHBTC",
				"c": "123",
				"X": "NEW",
				"i": 4293153,
				"l": "0.0",
				"z": "0.0",
				"T": 1499405658657,
				"t": -1
			}`),
			expectEvent:   true,
			expectedTopic: event.TopicEventOrderAccepted,
		},
		{
			name: "balanceUpdate (unhandled)",
			eventData: []byte(`{
				"e": "balanceUpdate",
				"E": 1573200697110,
				"a": "BTC",
				"d": "100.00000000",
				"T": 1573200697068
			}`),
			expectEvent: false,
		},
		{
			name: "listStatus (unhandled)",
			eventData: []byte(`{
				"e": "listStatus",
				"E": 1564035303637,
				"s": "ETHBTC",
				"g": 2,
				"c": "OCO"
			}`),
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear event bus
			for {
				ok := eb.Poll(func(e msgbus.Event) {})
				if !ok {
					break
				}
			}

			client.processUserDataStreamEvent(tt.eventData)

			var receivedEvent msgbus.Event
			ok := eb.Poll(func(e msgbus.Event) {
				receivedEvent = e
			})

			if tt.expectEvent && !ok {
				t.Errorf("Expected event to be published for %s", tt.name)
			}

			if !tt.expectEvent && ok {
				t.Errorf("Did not expect event to be published for %s", tt.name)
			}

			if tt.expectEvent && ok && receivedEvent.Ref.Topic != tt.expectedTopic {
				t.Errorf("Expected topic %v, got %v", tt.expectedTopic, receivedEvent.Ref.Topic)
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

// testConfig is a minimal config struct for integration tests
type testConfig struct {
	Catalog catalog.Config `yaml:"catalog"`
}

func loadTestConfig(path string) (*testConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg testConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// loadBinanceTestAccount builds a Binance account with an ED25519 API key from
// the catalog accounts config (env vars expanded). Skips the test if none found.
func loadBinanceTestAccount(t *testing.T) (*catalog.Account, string) {
	t.Helper()
	cfg, err := loadTestConfig("../../config/xarb.yml")
	if err != nil {
		t.Skipf("Skipping test: failed to load config: %v", err)
	}

	for i, ac := range cfg.Catalog.Accounts {
		if !strings.EqualFold(ac.Exchange, "binance") {
			continue
		}
		account := &catalog.Account{
			ID:       i + 1,
			UID:      i + 1,
			ExchID:   common.ExchangeBinance,
			Exchange: ac.Exchange,
			Name:     ac.Name,
		}
		for j, kc := range ac.APIKeys {
			if !strings.EqualFold(kc.Type, string(catalog.APITypeED25519)) {
				continue
			}
			key := os.ExpandEnv(kc.Key)
			secret := os.ExpandEnv(kc.Secret)
			if key == "" || secret == "" {
				continue
			}
			account.SetAPIKeys([]catalog.APIKey{{
				ID:      j + 1,
				UID:     account.UID,
				ExchID:  common.ExchangeBinance,
				Name:    kc.Name,
				APIType: catalog.APITypeED25519,
				Key:     key,
				Secret:  secret,
			}})
			return account, kc.Name
		}
	}

	t.Skip("Skipping test: no Binance account with ED25519 key found in config")
	return nil, ""
}

// TestIntegration_ReqBalanceSnapshot tests requesting balance snapshot via WebSocket API
// This test requires a valid Ed25519 API key configured in the catalog
func TestIntegration_ReqBalanceSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetAccount, targetAPIKeyName := loadBinanceTestAccount(t)

	t.Logf("Found account: ID=%d, Name=%s, APIKey=%s", targetAccount.ID, targetAccount.Name, targetAPIKeyName)

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	accountsMap := make(map[int]catalog.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	client, err := NewBinanceSpotExecutionClient(cat, eb, targetAccount.ID, targetAPIKeyName, 0)
	if err != nil {
		t.Fatalf("Failed to create execution client: %v", err)
	}

	// Connect to WebSocket API
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Binance WebSocket API")

	// Wait a moment for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Request balance snapshot
	err = client.ReqBalanceSnapshot(common.WalletTypeSpot)
	if err != nil {
		t.Fatalf("Failed to request balance snapshot: %v", err)
	}

	t.Log("Requested balance snapshot, waiting for response...")

	// Wait for response
	timeout := time.After(10 * time.Second)
	receivedSnapshot := false

loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			ok := eb.Poll(func(e msgbus.Event) {
				if e.Ref.Topic == event.TopicEventRespBalanceSnapshot {
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					snapshot, err := event.NewRespBalanceSnapshotFromBytes(buf)
					if err != nil {
						t.Fatalf("decode failed: %v", err)
					}
					t.Logf("Received balance snapshot: AccountID=%d, Balances=%d",
						snapshot.AccountID, len(snapshot.Balances))
					for i, b := range snapshot.Balances {
						if i < 10 { // Only log first 10 balances
							t.Logf("  Balance[%d]: TokenID=%d, Available=%.8f, Locked=%.8f, Total=%.8f",
								i, b.TokenID, b.Available, b.Locked, b.Total)
						}
					}
					receivedSnapshot = true
				}
			})
			if !ok {
				time.Sleep(50 * time.Millisecond)
			}
			if receivedSnapshot {
				break loop
			}
		}
	}

	if !receivedSnapshot {
		t.Error("Did not receive balance snapshot within timeout")
	}
}

// TestIntegration_ConnectAndPing tests basic WebSocket connection and keepalive
func TestIntegration_ConnectAndPing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetAccount, targetAPIKeyName := loadBinanceTestAccount(t)

	t.Logf("Found account: ID=%d, Name=%s", targetAccount.ID, targetAccount.Name)

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	accountsMap := make(map[int]catalog.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	client, err := NewBinanceSpotExecutionClient(cat, eb, targetAccount.ID, targetAPIKeyName, 0)
	if err != nil {
		t.Fatalf("Failed to create execution client: %v", err)
	}

	// Connect to WebSocket API
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	t.Log("Successfully connected to Binance WebSocket API")

	// Keep connection alive for a few seconds
	time.Sleep(3 * time.Second)

	// Verify still connected
	if !client.connected.Load() {
		t.Error("Connection dropped unexpectedly")
	} else {
		t.Log("Connection stable after 3 seconds")
	}

	// Disconnect
	client.Disconnect()

	if client.connected.Load() {
		t.Error("Failed to disconnect")
	} else {
		t.Log("Successfully disconnected")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// setPrivateFieldExec sets a private field on a struct using reflection
func setPrivateFieldExec(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}
