package bybit

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/msgbus"
	"gopkg.in/yaml.v3"
)

// testConfig mirrors the config structure for loading test config
type testConfig struct {
	Catalog struct {
		BaseURL  string `yaml:"base_url"`
		APIToken string `yaml:"api_token"`
	} `yaml:"catalog"`
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

func setPrivateFieldExec(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}

// TestIntegration_BybitPrivateStream tests the Bybit private stream connection and authentication
func TestIntegration_BybitPrivateStream(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config to get cpanel credentials
	cfg, err := loadTestConfig("../../../config/xarb.yml")
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

	// Find the bybit_lynxapp account (id=19)
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].ID == 19 || accounts[i].Name == "bybit_lynxapp" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: bybit_lynxapp account not found")
	}

	if targetAccount.APIType != cpanel.APITypeHMAC {
		t.Logf("Warning: account API type is %s, expected HMAC", targetAccount.APIType)
	}

	t.Logf("Found account: ID=%d, Name=%s, APIType=%s, Exchange=%s",
		targetAccount.ID, targetAccount.Name, targetAccount.APIType, targetAccount.Exchange)

	// Create event bus and catalog
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create private stream client
	client, err := NewBybitPrivateStreamClient(cat, eb, targetAccount.ID)
	if err != nil {
		t.Fatalf("Failed to create private stream client: %v", err)
	}

	// Connect to WebSocket
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Bybit Private Stream WebSocket")

	// Wait for authentication
	time.Sleep(2 * time.Second)

	if !client.authenticated.Load() {
		t.Error("Failed to authenticate to Bybit private stream")
	} else {
		t.Log("Successfully authenticated to Bybit private stream")
	}

	// Subscribe to order updates
	err = client.SubscribeOrderUpdate()
	if err != nil {
		t.Errorf("Failed to subscribe to order updates: %v", err)
	}

	// Subscribe to executions
	err = client.SubscribeExecution()
	if err != nil {
		t.Errorf("Failed to subscribe to executions: %v", err)
	}

	// Subscribe to wallet
	err = client.SubscribeWallet()
	if err != nil {
		t.Errorf("Failed to subscribe to wallet: %v", err)
	}

	t.Log("Subscribed to order, execution, and wallet topics")

	// Wait a moment for subscription confirmation
	time.Sleep(2 * time.Second)

	// Verify connection is still alive
	if !client.connected.Load() {
		t.Error("Connection dropped unexpectedly")
	} else {
		t.Log("Connection stable after subscriptions")
	}

	// Disconnect
	client.Disconnect()

	if client.connected.Load() {
		t.Error("Failed to disconnect")
	} else {
		t.Log("Successfully disconnected")
	}
}

// TestIntegration_BybitOrderEntry tests the Bybit order entry connection and authentication
func TestIntegration_BybitOrderEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config to get cpanel credentials
	cfg, err := loadTestConfig("../../../config/xarb.yml")
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

	// Find the bybit_lynxapp account (id=19)
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].ID == 19 || accounts[i].Name == "bybit_lynxapp" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: bybit_lynxapp account not found")
	}

	t.Logf("Found account: ID=%d, Name=%s, APIType=%s, Exchange=%s",
		targetAccount.ID, targetAccount.Name, targetAccount.APIType, targetAccount.Exchange)

	// Create event bus and catalog
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create order entry client
	client, err := NewBybitOrderEntryClient(cat, eb, targetAccount.ID)
	if err != nil {
		t.Fatalf("Failed to create order entry client: %v", err)
	}

	// Connect to WebSocket
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Bybit Order Entry WebSocket")

	// Wait for authentication
	time.Sleep(2 * time.Second)

	if !client.authenticated.Load() {
		t.Error("Failed to authenticate to Bybit order entry")
	} else {
		t.Log("Successfully authenticated to Bybit order entry")
	}

	// Verify connection is still alive
	if !client.connected.Load() {
		t.Error("Connection dropped unexpectedly")
	} else {
		t.Log("Connection stable after authentication")
	}

	// Disconnect
	client.Disconnect()

	if client.connected.Load() {
		t.Error("Failed to disconnect")
	} else {
		t.Log("Successfully disconnected")
	}
}

// TestIntegration_BybitExecutionClient tests the unified Bybit execution client
func TestIntegration_BybitExecutionClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config to get cpanel credentials
	cfg, err := loadTestConfig("../../../config/xarb.yml")
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

	// Find the bybit_lynxapp account (id=19)
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].ID == 19 || accounts[i].Name == "bybit_lynxapp" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: bybit_lynxapp account not found")
	}

	t.Logf("Found account: ID=%d, Name=%s, APIType=%s, Exchange=%s",
		targetAccount.ID, targetAccount.Name, targetAccount.APIType, targetAccount.Exchange)

	// Create event bus and catalog
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create unified execution client
	client, err := NewBybitExecutionClient(cat, eb, targetAccount.ID)
	if err != nil {
		t.Fatalf("Failed to create execution client: %v", err)
	}

	// Connect both private stream and order entry
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Bybit WebSocket (both private stream and order entry)")

	// Wait for authentication
	time.Sleep(2 * time.Second)

	// Check both clients authenticated
	if !client.privateStream.authenticated.Load() {
		t.Error("Private stream failed to authenticate")
	} else {
		t.Log("Private stream authenticated successfully")
	}

	if !client.orderEntry.authenticated.Load() {
		t.Error("Order entry failed to authenticate")
	} else {
		t.Log("Order entry authenticated successfully")
	}

	// Subscribe to all private data streams using granular methods
	if err = client.SubscribeOrderUpdate(); err != nil {
		t.Errorf("Failed to subscribe to order updates: %v", err)
	}
	if err = client.SubscribeFill(); err != nil {
		t.Errorf("Failed to subscribe to fills: %v", err)
	}
	if err = client.SubscribeBalance(); err != nil {
		t.Errorf("Failed to subscribe to balance: %v", err)
	}

	t.Log("Subscribed to order, fill, and balance streams")

	// Wait a moment for subscription confirmation
	time.Sleep(2 * time.Second)

	// Verify connections are still alive
	if !client.privateStream.connected.Load() {
		t.Error("Private stream connection dropped unexpectedly")
	}
	if !client.orderEntry.connected.Load() {
		t.Error("Order entry connection dropped unexpectedly")
	}

	t.Log("Both connections stable after subscriptions")

	// Disconnect
	client.Disconnect()

	if client.privateStream.connected.Load() || client.orderEntry.connected.Load() {
		t.Error("Failed to disconnect")
	} else {
		t.Log("Successfully disconnected both clients")
	}
}

// TestIntegration_BybitWalletUpdate tests receiving wallet updates
func TestIntegration_BybitWalletUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config to get cpanel credentials
	cfg, err := loadTestConfig("../../../config/xarb.yml")
	if err != nil {
		t.Skipf("Skipping test: failed to load config: %v", err)
	}

	// Create cpanel client and fetch accounts
	cpanelClient := cpanel.NewCpanelClient(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	accounts, err := cpanelClient.GetAccounts(ctx)
	if err != nil {
		t.Skipf("Skipping test: failed to get accounts: %v", err)
	}

	// Find the bybit_lynxapp account (id=19)
	var targetAccount *cpanel.Account
	for i := range accounts {
		if accounts[i].ID == 19 || accounts[i].Name == "bybit_lynxapp" {
			targetAccount = &accounts[i]
			break
		}
	}

	if targetAccount == nil {
		t.Skip("Skipping test: bybit_lynxapp account not found")
	}

	t.Logf("Found account: ID=%d, Name=%s", targetAccount.ID, targetAccount.Name)

	// Create event bus and catalog
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject account into catalog
	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateFieldExec(cat, "accounts", accountsMap)

	// Create unified execution client
	client, err := NewBybitExecutionClient(cat, eb, targetAccount.ID)
	if err != nil {
		t.Fatalf("Failed to create execution client: %v", err)
	}

	// Connect
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Bybit WebSocket")

	// Wait for authentication
	time.Sleep(2 * time.Second)

	// Subscribe to balance updates only (for this test)
	err = client.SubscribeBalance()
	if err != nil {
		t.Fatalf("Failed to subscribe to balance updates: %v", err)
	}

	t.Log("Subscribed to balance updates, listening for wallet updates...")
	t.Log("Note: Wallet updates are pushed when there are balance changes")
	t.Log("Waiting 10 seconds for any wallet updates...")

	// Poll for wallet updates
	timeout := time.After(10 * time.Second)
	receivedUpdate := false

loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			ok := eb.Poll(func(e msgbus.Event) {
				if e.Ref.Topic == event.TopicEventBalanceUpdate {
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					update := msgbus.DeserializeBalanceUpdate(buf)
					t.Logf("Received balance update: AccountID=%d, Balances=%d",
						update.AccountID, len(update.Balances))
					for i, b := range update.Balances {
						if i < 10 { // Only log first 10 balances
							t.Logf("  Balance[%d]: TokenID=%d, Available=%.8f, Locked=%.8f, Total=%.8f",
								i, b.TokenID, b.Available, b.Locked, b.Total)
						}
					}
					receivedUpdate = true
				}
			})
			if !ok {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	if !receivedUpdate {
		t.Log("No wallet updates received during test period (this may be normal if no balance changes occurred)")
	}
}

// TestUnit_HMACSignature tests the HMAC signature generation
func TestUnit_HMACSignature(t *testing.T) {
	// Create a mock client to test signing
	client := &BybitPrivateStreamClient{
		account: cpanel.Account{
			APIKey:    "test_api_key",
			APISecret: "test_api_secret",
		},
	}

	// Test signature generation
	payload := []byte("GET/realtime1234567890")
	signature := client.signHMAC(payload)

	// Verify signature is not empty
	if signature == "" {
		t.Error("Signature should not be empty")
	}

	// Verify signature is hex-encoded (length should be 64 for SHA256)
	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	t.Logf("Generated signature: %s", signature)
}

// TestUnit_OrderEntrySignature tests the order entry HMAC signature generation
func TestUnit_OrderEntrySignature(t *testing.T) {
	// Create a mock client to test signing
	client := &BybitOrderEntryClient{
		account: cpanel.Account{
			APIKey:    "test_api_key",
			APISecret: "test_api_secret",
		},
	}

	// Test signature generation
	payload := client.buildSignPayload(1234567890000, 1)
	signature := client.signHMAC(payload)

	// Verify signature is not empty
	if signature == "" {
		t.Error("Signature should not be empty")
	}

	// Verify signature is hex-encoded (length should be 64 for SHA256)
	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	t.Logf("Sign payload: %s", string(payload))
	t.Logf("Generated signature: %s", signature)
}
