package binance

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
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
		account: cpanel.Account{
			APIKey: "test-api-key",
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
		account: cpanel.Account{
			APIKey: "test-api-key",
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
				tt.price, tt.quantity, 1234567890000, "test-sig", 1,
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
	eb := evbus.NewEventBus()
	client := &BinanceSpotExecutionClient{
		eventBus:  eb,
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
	var receivedEvent evbus.Event
	ok := eb.Poll(func(e evbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected balance snapshot event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventReqBalanceSnapshot {
		t.Errorf("Expected TopicEventReqBalanceSnapshot, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	snapshot := evbus.DeserializeReqBalanceSnapshot(buf)

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
	eb := evbus.NewEventBus()
	client := &BinanceSpotExecutionClient{
		eventBus: eb,
	}

	// Mock Binance order response
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
	var receivedEvent evbus.Event
	ok := eb.Poll(func(e evbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected order update event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventOrderUpdate {
		t.Errorf("Expected TopicEventOrderUpdate, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	orderUpdate := evbus.DeserializeOrderUpdate(buf)

	if orderUpdate.OrderID != 12345678 {
		t.Errorf("Expected OrderID 12345678, got %d", orderUpdate.OrderID)
	}

	if orderUpdate.ClientOrderID != 123 {
		t.Errorf("Expected ClientOrderID 123, got %d", orderUpdate.ClientOrderID)
	}

	if orderUpdate.OrderStatus != common.OrderStatusAccepted {
		t.Errorf("Expected OrderStatusAccepted, got %v", orderUpdate.OrderStatus)
	}
}

func TestParseOrderStatus(t *testing.T) {
	client := &BinanceSpotExecutionClient{}

	tests := []struct {
		input    string
		expected common.OrderStatus
	}{
		{"NEW", common.OrderStatusAccepted},
		{"PARTIALLY_FILLED", common.OrderStatusPartiallyFilled},
		{"FILLED", common.OrderStatusFilled},
		{"CANCELED", common.OrderStatusCanceled},
		{"REJECTED", common.OrderStatusRejected},
		{"EXPIRED", common.OrderStatusCanceled},
		{"UNKNOWN", common.OrderStatusUninitialized},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := client.parseOrderStatus(tt.input)
			if result != tt.expected {
				t.Errorf("parseOrderStatus(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

// TestIntegration_ReqBalanceSnapshot tests requesting balance snapshot via WebSocket API
// This test requires a valid Ed25519 API key configured in the catalog
func TestIntegration_ReqBalanceSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config to get cpanel credentials
	cfg, err := strategy.LoadConfig("../../../config/xarb.yml")
	if err != nil {
		t.Skipf("Skipping test: failed to load config: %v", err)
	}

	// Create cpanel client and fetch accounts
	cpanelClient := cpanel.NewCpanelClient(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accounts, err := cpanelClient.GetAccounts(ctx)
	if err != nil {
		t.Skipf("Skipping test: failed to get accounts: %v", err)
	}

	// Find the lynxapp_ed25519 account
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].Name == "lynxapp_ed25519" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: lynxapp_ed25519 account not found")
	}

	if targetAccount.APIType != cpanel.APITypeED25519 {
		t.Skipf("Skipping test: account API type is %s, expected ED25519", targetAccount.APIType)
	}

	t.Logf("Found account: ID=%d, Name=%s, APIType=%s", targetAccount.ID, targetAccount.Name, targetAccount.APIType)

	// Create event bus and catalog
	eb := evbus.NewEventBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog (copy the account, not pointer)
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create execution client
	client, err := NewBinanceSpotExecutionClient(cat, eb, targetAccount.ID)
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
	err = client.ReqBalanceSnapshot()
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
			ok := eb.Poll(func(e evbus.Event) {
				if e.Ref.Topic == event.TopicEventReqBalanceSnapshot {
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					snapshot := evbus.DeserializeReqBalanceSnapshot(buf)
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

	// Load config to get cpanel credentials
	cfg, err := strategy.LoadConfig("../../../config/xarb.yml")
	if err != nil {
		t.Skipf("Skipping test: failed to load config: %v", err)
	}

	// Create cpanel client and fetch accounts
	cpanelClient := cpanel.NewCpanelClient(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accounts, err := cpanelClient.GetAccounts(ctx)
	if err != nil {
		t.Skipf("Skipping test: failed to get accounts: %v", err)
	}

	// Find the lynxapp_ed25519 account
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].Name == "lynxapp_ed25519" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: lynxapp_ed25519 account not found")
	}

	t.Logf("Found account: ID=%d, Name=%s", targetAccount.ID, targetAccount.Name)

	// Create event bus and catalog
	eb := evbus.NewEventBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog (copy the account, not pointer)
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create execution client
	client, err := NewBinanceSpotExecutionClient(cat, eb, targetAccount.ID)
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
