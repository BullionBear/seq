package bybit

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/bytedance/sonic"
)

// Note: testConfig and loadTestConfig are defined in execclient_test.go

func TestUnmarshalReqDepthSnapshot(t *testing.T) {
	// Setup
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{} // Mock catalog not needed for unmarshal
	client := NewBybitHTTPClient(cat, eb)

	// Bybit response format
	jsonBody := []byte(`{
  "retCode": 0,
  "retMsg": "OK",
  "result": {
    "s": "BTCUSDT",
    "a": [
      ["65557.7", "16.606555"],
      ["65558.0", "10.5"]
    ],
    "b": [
      ["65485.47", "47.081829"],
      ["65480.00", "20.0"],
      ["65475.50", "15.5"]
    ],
    "ts": 1716863719031,
    "u": 230704,
    "seq": 1432604333,
    "cts": 1716863718905
  },
  "retExtInfo": {},
  "time": 1716863719382
}`)

	var depth event.RespDepthSnapshot
	err := client.unmarshalRespDepthSnapshot(jsonBody, &depth)
	if err != nil {
		t.Fatalf("unmarshalRespDepthSnapshot failed: %v", err)
	}

	if depth.DepthID != 230704 {
		t.Errorf("Expected DepthID 230704, got %d", depth.DepthID)
	}

	// Bybit returns timestamp in milliseconds, we convert to nanoseconds
	expectedTs := uint64(1716863719031 * 1000000)
	if depth.Timestamp != expectedTs {
		t.Errorf("Expected Timestamp %d, got %d", expectedTs, depth.Timestamp)
	}

	if len(depth.Asks) != 2 {
		t.Fatalf("Expected 2 asks, got %d", len(depth.Asks))
	}
	if depth.Asks[0].Price != 65557.7 || depth.Asks[0].Quantity != 16.606555 {
		t.Errorf("Ask[0] mismatch: got %v", depth.Asks[0])
	}
	if depth.Asks[1].Price != 65558.0 || depth.Asks[1].Quantity != 10.5 {
		t.Errorf("Ask[1] mismatch: got %v", depth.Asks[1])
	}

	if len(depth.Bids) != 3 {
		t.Fatalf("Expected 3 bids, got %d", len(depth.Bids))
	}
	if depth.Bids[0].Price != 65485.47 || depth.Bids[0].Quantity != 47.081829 {
		t.Errorf("Bid[0] mismatch: got %v", depth.Bids[0])
	}
	if depth.Bids[1].Price != 65480.00 || depth.Bids[1].Quantity != 20.0 {
		t.Errorf("Bid[1] mismatch: got %v", depth.Bids[1])
	}
	if depth.Bids[2].Price != 65475.50 || depth.Bids[2].Quantity != 15.5 {
		t.Errorf("Bid[2] mismatch: got %v", depth.Bids[2])
	}
}

func TestUnmarshalReqDepthSnapshot_APIError(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	client := NewBybitHTTPClient(cat, eb)

	// Bybit error response
	jsonBody := []byte(`{
  "retCode": 10001,
  "retMsg": "Invalid symbol",
  "result": {},
  "retExtInfo": {},
  "time": 1716863719382
}`)

	var depth event.RespDepthSnapshot
	err := client.unmarshalRespDepthSnapshot(jsonBody, &depth)
	if err == nil {
		t.Fatal("Expected error for non-zero retCode")
	}

	expectedErr := "Bybit API error: Invalid symbol"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func BenchmarkUnmarshalReqDepthSnapshot(b *testing.B) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	client := NewBybitHTTPClient(cat, eb)

	jsonBody := []byte(`{
  "retCode": 0,
  "retMsg": "OK",
  "result": {
    "s": "BTCUSDT",
    "a": [
      ["65557.7", "16.606555"],
      ["65558.0", "10.5"],
      ["65559.0", "20.0"],
      ["65560.0", "30.0"],
      ["65561.0", "40.0"]
    ],
    "b": [
      ["65485.47", "47.081829"],
      ["65480.00", "20.0"],
      ["65475.50", "15.5"],
      ["65470.00", "25.0"],
      ["65465.00", "35.0"]
    ],
    "ts": 1716863719031,
    "u": 230704,
    "seq": 1432604333,
    "cts": 1716863718905
  },
  "retExtInfo": {},
  "time": 1716863719382
}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var depth event.RespDepthSnapshot
		err := client.unmarshalRespDepthSnapshot(jsonBody, &depth)
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

func BenchmarkSonicUnmarshal(b *testing.B) {
	jsonBody := []byte(`{
  "retCode": 0,
  "retMsg": "OK",
  "result": {
    "s": "BTCUSDT",
    "a": [
      ["65557.7", "16.606555"],
      ["65558.0", "10.5"],
      ["65559.0", "20.0"],
      ["65560.0", "30.0"],
      ["65561.0", "40.0"]
    ],
    "b": [
      ["65485.47", "47.081829"],
      ["65480.00", "20.0"],
      ["65475.50", "15.5"],
      ["65470.00", "25.0"],
      ["65465.00", "35.0"]
    ],
    "ts": 1716863719031,
    "u": 230704,
    "seq": 1432604333,
    "cts": 1716863718905
  },
  "retExtInfo": {},
  "time": 1716863719382
}`)

	// Local struct with tags for Sonic
	type SonicPriceLevel []string
	type SonicResult struct {
		S   string            `json:"s"`
		A   []SonicPriceLevel `json:"a"`
		B   []SonicPriceLevel `json:"b"`
		Ts  int64             `json:"ts"`
		U   int               `json:"u"`
		Seq int64             `json:"seq"`
		Cts int64             `json:"cts"`
	}
	type SonicResponse struct {
		RetCode    int         `json:"retCode"`
		RetMsg     string      `json:"retMsg"`
		Result     SonicResult `json:"result"`
		RetExtInfo interface{} `json:"retExtInfo"`
		Time       int64       `json:"time"`
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var resp SonicResponse
		err := sonic.Unmarshal(jsonBody, &resp)
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

func TestReqDepthSnapshot_Integration(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		query := r.URL.Query()
		if query.Get("category") != "spot" {
			t.Errorf("Expected category spot, got %s", query.Get("category"))
		}
		if query.Get("symbol") != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", query.Get("symbol"))
		}
		if query.Get("limit") != "50" {
			t.Errorf("Expected limit 50, got %s", query.Get("limit"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
  "retCode": 0,
  "retMsg": "OK",
  "result": {
    "s": "BTCUSDT",
    "a": [["65557.7", "16.606555"]],
    "b": [["65485.47", "47.081829"]],
    "ts": 1716863719031,
    "u": 230704,
    "seq": 1432604333,
    "cts": 1716863718905
  },
  "retExtInfo": {},
  "time": 1716863719382
}`))
	}))
	defer server.Close()

	// Setup Dependencies
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject Catalog Data
	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: cpanel.Product{
			ID:   1,
			Name: "Spot",
			Slug: "spot",
		},
	}

	setPrivateField(cat, "symbols", symbols)

	// Create Client with Test Server URL
	client := NewBybitHTTPClient(cat, eb)

	// Inject baseURL (private field)
	setPrivateField(&client, "baseURL", server.URL)

	// Execute
	err := client.ReqDepthSnapshot(1, 50)
	if err != nil {
		t.Fatalf("ReqDepthSnapshot failed: %v", err)
	}

	// Verify event was published
	var receivedEvent msgbus.Event
	eb.Register("test", nil, func(ev msgbus.Event) {
		receivedEvent = ev
	})
	if !eb.Dispatch() {
		t.Fatal("Expected event to be dispatched")
	}

	if receivedEvent.Ref.Topic != event.TopicEventDepthSnapshot {
		t.Errorf("Expected TopicEventDepthSnapshot, got %d", receivedEvent.Ref.Topic)
	}
}

func TestReqDepthSnapshot_LinearCategory(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("category") != "linear" {
			t.Errorf("Expected category linear, got %s", query.Get("category"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
  "retCode": 0,
  "retMsg": "OK",
  "result": {
    "s": "BTCUSDT",
    "a": [["65557.7", "16.606555"]],
    "b": [["65485.47", "47.081829"]],
    "ts": 1716863719031,
    "u": 230704
  },
  "retExtInfo": {},
  "time": 1716863719382
}`))
	}))
	defer server.Close()

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: cpanel.Product{
			ID:   2,
			Name: "Linear",
			Slug: "linear", // USDT perpetual
		},
	}

	setPrivateField(cat, "symbols", symbols)

	client := NewBybitHTTPClient(cat, eb)
	setPrivateField(&client, "baseURL", server.URL)

	err := client.ReqDepthSnapshot(1, 100)
	if err != nil {
		t.Fatalf("ReqDepthSnapshot failed: %v", err)
	}
}

func setPrivateField(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}

// DataClient tests

func TestDataClient_SubscribeDepthUpdate(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: cpanel.Product{
			ID:   1,
			Name: "Spot",
			Slug: "spot",
		},
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBybitDataClient(cat, eb)

	// Test subscription
	client.SubscribeDepthUpdate(1, 50, 0)

	if !client.HasSub() {
		t.Error("Expected HasSub to return true after subscription")
	}
}

func TestDataClient_ProcessOrderbookSnapshot(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: cpanel.Product{
			ID:   1,
			Name: "Spot",
			Slug: "spot",
		},
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBybitDataClient(cat, eb)

	// Register topic mapping
	topicToSymbol := make(map[string]int)
	topicToSymbol["orderbook.50.BTCUSDT"] = 1
	setPrivateField(client, "topicToSymbol", topicToSymbol)

	// Bybit orderbook snapshot message
	msg := []byte(`{
		"topic": "orderbook.50.BTCUSDT",
		"type": "snapshot",
		"ts": 1672304484978,
		"data": {
			"s": "BTCUSDT",
			"b": [
				["16493.50", "0.006"],
				["16493.00", "0.100"]
			],
			"a": [
				["16611.00", "0.029"],
				["16612.00", "0.213"]
			],
			"u": 18521288,
			"seq": 7961638724
		},
		"cts": 1672304484976
	}`)

	// Process the message
	client.processMessage(msg)

	// Verify event was published
	var receivedEvent msgbus.Event
	eb.Register("test", nil, func(ev msgbus.Event) {
		receivedEvent = ev
	})
	if !eb.Dispatch() {
		t.Fatal("Expected event to be dispatched")
	}

	if receivedEvent.Ref.Topic != event.TopicEventDepthSnapshot {
		t.Errorf("Expected TopicEventDepthSnapshot, got %d", receivedEvent.Ref.Topic)
	}

	// Deserialize and verify
	snapshot := event.NewDepthSnapshotFromBytes(eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length))
	if snapshot.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", snapshot.SymbolID)
	}
	if snapshot.DepthID != 18521288 {
		t.Errorf("Expected DepthID 18521288, got %d", snapshot.DepthID)
	}
	if len(snapshot.Bids) != 2 {
		t.Errorf("Expected 2 bids, got %d", len(snapshot.Bids))
	}
	if len(snapshot.Asks) != 2 {
		t.Errorf("Expected 2 asks, got %d", len(snapshot.Asks))
	}
	if snapshot.Bids[0].Price != 16493.50 {
		t.Errorf("Expected bid price 16493.50, got %f", snapshot.Bids[0].Price)
	}
	if snapshot.Asks[0].Price != 16611.00 {
		t.Errorf("Expected ask price 16611.00, got %f", snapshot.Asks[0].Price)
	}
}

func TestDataClient_ProcessOrderbookDelta(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: cpanel.Product{
			ID:   1,
			Name: "Linear",
			Slug: "linear",
		},
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBybitDataClient(cat, eb)

	// Register topic mapping
	topicToSymbol := make(map[string]int)
	topicToSymbol["orderbook.50.BTCUSDT"] = 1
	setPrivateField(client, "topicToSymbol", topicToSymbol)

	// Bybit orderbook delta message
	msg := []byte(`{
		"topic": "orderbook.50.BTCUSDT",
		"type": "delta",
		"ts": 1687940967466,
		"data": {
			"s": "BTCUSDT",
			"b": [
				["30247.20", "30.028"],
				["30240.00", "0"]
			],
			"a": [
				["30248.70", "0"],
				["30249.30", "0.892"]
			],
			"u": 177400507,
			"seq": 66544703342
		},
		"cts": 1687940967464
	}`)

	// Process the message
	client.processMessage(msg)

	// Verify event was published
	var receivedEvent msgbus.Event
	eb.Register("test", nil, func(ev msgbus.Event) {
		receivedEvent = ev
	})
	if !eb.Dispatch() {
		t.Fatal("Expected event to be dispatched")
	}

	if receivedEvent.Ref.Topic != event.TopicEventDepthUpdate {
		t.Errorf("Expected TopicEventDepthUpdate, got %d", receivedEvent.Ref.Topic)
	}

	// Deserialize and verify
	update := event.NewDepthUpdateFromBytes(eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length))
	if update.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", update.SymbolID)
	}
	if update.DepthID != 177400507 {
		t.Errorf("Expected DepthID 177400507, got %d", update.DepthID)
	}
	if len(update.Bids) != 2 {
		t.Errorf("Expected 2 bids, got %d", len(update.Bids))
	}
	if len(update.Asks) != 2 {
		t.Errorf("Expected 2 asks, got %d", len(update.Asks))
	}
}

func TestCategoryMapping(t *testing.T) {
	tests := []struct {
		productSlug string
		expected    Category
		shouldExist bool
	}{
		{"spot", CategorySpot, true},
		{"linear", CategoryLinear, true},
		{"inverse", CategoryInverse, true},
		{"option", CategoryOption, true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		category, ok := ProductSlugToCategory[tt.productSlug]
		if ok != tt.shouldExist {
			t.Errorf("ProductSlugToCategory[%s] existence = %v, want %v", tt.productSlug, ok, tt.shouldExist)
		}
		if ok && category != tt.expected {
			t.Errorf("ProductSlugToCategory[%s] = %v, want %v", tt.productSlug, category, tt.expected)
		}
	}
}

func TestCategoryToWsURL(t *testing.T) {
	tests := []struct {
		category Category
		expected string
	}{
		{CategorySpot, WsURLSpot},
		{CategoryLinear, WsURLLinear},
		{CategoryInverse, WsURLInverse},
		{CategoryOption, WsURLOption},
	}

	for _, tt := range tests {
		url, ok := CategoryToWsURL[tt.category]
		if !ok {
			t.Errorf("CategoryToWsURL[%s] not found", tt.category)
		}
		if url != tt.expected {
			t.Errorf("CategoryToWsURL[%s] = %v, want %v", tt.category, url, tt.expected)
		}
	}
}

// ============================================================================
// ReqBalanceSnapshot Tests
// ============================================================================

// TestIntegration_ReqBalanceSnapshot tests requesting balance snapshot via HTTP API
// Uses account id=19 (bybit_lynxapp) with HMAC authentication
func TestIntegration_ReqBalanceSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	targetAccount, apiKeyName := loadBybitTestAccount(t)

	t.Logf("Found account: ID=%d, Name=%s, Exchange=%s",
		targetAccount.ID, targetAccount.Name, targetAccount.Exchange)

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	accountsMap := make(map[int]cpanel.Account)
	accountsMap[targetAccount.ID] = *targetAccount
	setPrivateField(cat, "accounts", accountsMap)

	client := NewBybitHTTPClient(cat, eb)

	err := client.ReqBalanceSnapshot(targetAccount.ID, 0, apiKeyName, "UNIFIED")
	if err != nil {
		t.Fatalf("Failed to request balance snapshot: %v", err)
	}

	t.Log("Requested balance snapshot successfully")

	// Check if event was published
	var receivedEvent msgbus.Event
	eb.Register("test", nil, func(ev msgbus.Event) {
		receivedEvent = ev
	})

	if eb.Dispatch() {
		if receivedEvent.Ref.Topic == event.TopicEventRespBalanceSnapshot {
			buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
			snapshot := event.NewRespBalanceSnapshotFromBytes(buf)
			t.Logf("Received balance snapshot: AccountID=%d, Balances=%d",
				snapshot.AccountID, len(snapshot.Balances))
			for i, b := range snapshot.Balances {
				if i < 10 { // Only log first 10 balances
					t.Logf("  Balance[%d]: TokenID=%d, Available=%.8f, Locked=%.8f, Total=%.8f",
						i, b.TokenID, b.Available, b.Locked, b.Total)
				}
			}
		}
	} else {
		t.Log("No balance event published (account may have zero balances)")
	}
}

// TestUnit_SignHMAC tests the HMAC signature generation
func TestUnit_SignHMAC(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	client := NewBybitHTTPClient(cat, eb)

	// Test with known values
	payload := []byte("1234567890test_api_key5000accountType=UNIFIED")
	secret := "test_secret"

	signature := client.signHMAC(payload, secret)

	// Verify signature is not empty
	if signature == "" {
		t.Error("Signature should not be empty")
	}

	// Verify signature is hex-encoded (length should be 64 for SHA256)
	if len(signature) != 64 {
		t.Errorf("Expected signature length 64, got %d", len(signature))
	}

	t.Logf("Payload: %s", string(payload))
	t.Logf("Signature: %s", signature)
}
