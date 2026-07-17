package binance

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/bytedance/sonic"
)

func TestUnmarshalReqDepthSnapshot(t *testing.T) {
	// Setup
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{} // Mock catalog not needed for unmarshal
	client := NewBinanceHTTPClient(cat, eb)

	jsonBody := []byte(`{
  "lastUpdateId": 1027024,
  "bids": [
    [
      "4.00000000",
      "431.00000000"
    ],
    [
       "3.00000000",
       "100.00000000"
    ]
  ],
  "asks": [
    [
      "4.00000200",
      "12.00000000"
    ]
  ]
}`)

	var depth event.RespDepthSnapshot
	err := client.unmarshalRespDepthSnapshot(jsonBody, &depth)
	if err != nil {
		t.Fatalf("unmarshalRespDepthSnapshot failed: %v", err)
	}

	if depth.DepthID != 1027024 {
		t.Errorf("Expected DepthID 1027024, got %d", depth.DepthID)
	}

	if len(depth.Bids) != 2 {
		t.Fatalf("Expected 2 bids, got %d", len(depth.Bids))
	}
	if depth.Bids[0].Price != 4.0 || depth.Bids[0].Quantity != 431.0 {
		t.Errorf("Bid[0] mismatch: got %v", depth.Bids[0])
	}
	if depth.Bids[1].Price != 3.0 || depth.Bids[1].Quantity != 100.0 {
		t.Errorf("Bid[1] mismatch: got %v", depth.Bids[1])
	}

	if len(depth.Asks) != 1 {
		t.Fatalf("Expected 1 ask, got %d", len(depth.Asks))
	}
	if depth.Asks[0].Price != 4.000002 || depth.Asks[0].Quantity != 12.0 {
		t.Errorf("Ask[0] mismatch: got %v", depth.Asks[0])
	}
}

func BenchmarkUnmarshalReqDepthSnapshot(b *testing.B) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	client := NewBinanceHTTPClient(cat, eb)

	jsonBody := []byte(`{
  "lastUpdateId": 1027024,
  "bids": [
    ["4.00000000", "431.00000000"],
    ["3.99000000", "100.00000000"],
    ["3.98000000", "200.00000000"],
    ["3.97000000", "300.00000000"],
    ["3.96000000", "400.00000000"]
  ],
  "asks": [
    ["4.01000000", "12.00000000"],
    ["4.02000000", "20.00000000"],
    ["4.03000000", "30.00000000"],
    ["4.04000000", "40.00000000"],
    ["4.05000000", "50.00000000"]
  ]
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
  "lastUpdateId": 1027024,
  "bids": [
    ["4.00000000", "431.00000000"],
    ["3.99000000", "100.00000000"],
    ["3.98000000", "200.00000000"],
    ["3.97000000", "300.00000000"],
    ["3.96000000", "400.00000000"]
  ],
  "asks": [
    ["4.01000000", "12.00000000"],
    ["4.02000000", "20.00000000"],
    ["4.03000000", "30.00000000"],
    ["4.04000000", "40.00000000"],
    ["4.05000000", "50.00000000"]
  ]
}`)

	// Local struct with tags for Sonic
	type SonicPriceLevel []string
	type SonicDepthSnapshot struct {
		LastUpdateId int               `json:"lastUpdateId"`
		Bids         []SonicPriceLevel `json:"bids"`
		Asks         []SonicPriceLevel `json:"asks"`
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var depth SonicDepthSnapshot
		err := sonic.Unmarshal(jsonBody, &depth)
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

func TestReqCreateOrder(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-MBX-APIKEY") != "test-api-key" {
			t.Errorf("Expected API key test-api-key, got %s", r.Header.Get("X-MBX-APIKEY"))
		}

		query := r.URL.Query()
		if query.Get("symbol") != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", query.Get("symbol"))
		}
		if query.Get("side") != "BUY" {
			t.Errorf("Expected side BUY, got %s", query.Get("side"))
		}
		if query.Get("quantity") != "1.5" {
			t.Errorf("Expected quantity 1.5, got %s", query.Get("quantity"))
		}
		if query.Get("signature") == "" {
			t.Error("Expected signature to be present")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"orderId": 12345}`))
	}))
	defer server.Close()

	// Setup Dependecies
	eb := *msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject Catalog Data
	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{
		ID:   1,
		Name: "BTCUSDT",
	}

	acct := catalog.Account{ID: 10}
	acct.SetAPIKeys([]catalog.APIKey{{
		ID:   1,
		Name: "test-key",
		Key:  "test-api-key",
		Secret: "test-secret",
	}})
	accounts := make(map[int]catalog.Account)
	accounts[10] = acct

	setPrivateField(cat, "symbols", symbols)
	setPrivateField(cat, "accounts", accounts)

	// Create Client with Test Server URL
	client := NewBinanceHTTPClient(cat, &eb)

	// Inject baseURL (private field)
	setPrivateField(&client, "baseURL", server.URL)

	// Execute
	err := client.ReqCreateOrder(10, "test-key", 1, common.OrderTypeLimit, common.SideBuy, common.TimeInForceGTC, 1.5, 50000.0)
	if err != nil {
		t.Fatalf("ReqCreateOrder failed: %v", err)
	}
}

func setPrivateField(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}

func TestReqCreateOrder_PostOnly(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("type") != "LIMIT_MAKER" {
			t.Errorf("Expected type LIMIT_MAKER, got %s", query.Get("type"))
		}
		if query.Get("timeInForce") != "" {
			t.Errorf("Expected empty timeInForce, got %s", query.Get("timeInForce"))
		}
		// Basic checks
		if query.Get("symbol") != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", query.Get("symbol"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"orderId": 123456}`))
	}))
	defer server.Close()

	eb := *msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}

	acct := catalog.Account{ID: 10}
	acct.SetAPIKeys([]catalog.APIKey{{
		ID:     1,
		Name:   "test-key",
		Key:    "k",
		Secret: "s",
	}})
	accounts := make(map[int]catalog.Account)
	accounts[10] = acct

	setPrivateField(cat, "symbols", symbols)
	setPrivateField(cat, "accounts", accounts)

	client := NewBinanceHTTPClient(cat, &eb)
	setPrivateField(&client, "baseURL", server.URL)

	// Execute with PO
	err := client.ReqCreateOrder(10, "test-key", 1, common.OrderTypeLimit, common.SideBuy, common.TimeInForcePO, 1.5, 50000.0)
	if err != nil {
		t.Fatalf("ReqCreateOrder failed: %v", err)
	}
}
