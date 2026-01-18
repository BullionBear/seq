package binance

import (
	"testing"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/BullionBear/seq/pkg/model"
	"github.com/bytedance/sonic"
)

func TestUnmarshalDepthSnapshot(t *testing.T) {
	// Setup
	eb := evbus.NewEventBus()
	cat := &catalog.Catalog{} // Mock catalog not needed for unmarshal
	client := NewBinanceHTTPClient(cat, &eb)

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

	var depth model.DepthSnapshot
	err := client.UnmarshalDepthSnapshot(jsonBody, &depth)
	if err != nil {
		t.Fatalf("UnmarshalDepthSnapshot failed: %v", err)
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

func BenchmarkUnmarshalDepthSnapshot(b *testing.B) {
	eb := evbus.NewEventBus()
	cat := &catalog.Catalog{}
	client := NewBinanceHTTPClient(cat, &eb)

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
		var depth model.DepthSnapshot
		err := client.UnmarshalDepthSnapshot(jsonBody, &depth)
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
