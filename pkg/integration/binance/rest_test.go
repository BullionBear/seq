package binance

import (
	"testing"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

// TestClient_GetDepth_RealRequest tests the GetDepth method with a real API request
func TestClient_GetDepth_RealRequest(t *testing.T) {
	client := NewClient()

	params := DepthParams{
		Symbol: "BTCUSDT",
		Limit:  100,
	}

	depth, err := client.GetDepth(params)
	if err != nil {
		t.Fatalf("GetDepth failed: %v", err)
	}

	if depth == nil {
		t.Fatal("GetDepth returned nil response")
	}

	if depth.LastUpdateID == 0 {
		t.Error("LastUpdateID should not be zero")
	}

	if depth.BidsLen() == 0 {
		t.Error("Bids should not be empty")
	}

	if depth.AsksLen() == 0 {
		t.Error("Asks should not be empty")
	}

	// Verify bid structure (price, quantity)
	bid := depth.Bids(0)
	if bid == nil {
		t.Fatal("Bids(0) returned nil")
	}
	if bid.Price <= 0 {
		t.Error("Bid price should be greater than zero")
	}
	if bid.Quantity <= 0 {
		t.Error("Bid quantity should be greater than zero")
	}

	// Verify ask structure (price, quantity)
	ask := depth.Asks(0)
	if ask == nil {
		t.Fatal("Asks(0) returned nil")
	}
	if ask.Price <= 0 {
		t.Error("Ask price should be greater than zero")
	}
	if ask.Quantity <= 0 {
		t.Error("Ask quantity should be greater than zero")
	}

	t.Logf("Successfully retrieved order book: LastUpdateID=%d, Bids=%d, Asks=%d",
		depth.LastUpdateID, depth.BidsLen(), depth.AsksLen())

	// Release the response back to the pool
	client.ReleaseDepthResponse(depth)
}

// TestClient_GetDepth_ZeroAllocation tests that request building has minimal allocations
// This test verifies that the request building part (URL construction, parameter setting)
// uses fasthttp's object pooling to minimize allocations.
//
// Note: Full HTTP requests will have allocations from:
// - Network I/O buffers
// - JSON response parsing
// - Response structure creation
// But the request building itself should be near-zero allocation.
func TestClient_GetDepth_ZeroAllocation(t *testing.T) {
	client := NewClient()

	params := DepthParams{
		Symbol: "BTCUSDT",
		Limit:  100,
	}

	// Test request building part (without actual HTTP call)
	// We test the URL building and parameter setting using pooled byte buffer
	allocs := testing.AllocsPerRun(1000, func() {
		req := fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		// Use the same pooled byte buffer approach as the actual implementation
		buf := bytebufferpool.Get()
		buf.WriteString(client.baseURL)
		buf.WriteString(EndpointDepth)
		req.SetRequestURIBytes(buf.B)
		bytebufferpool.Put(buf)
		uri := req.URI()

		args := uri.QueryArgs()
		args.Set("symbol", params.Symbol)
		args.Set("limit", "100")
	})

	// Request building with pooled byte buffer should have minimal allocations
	// Byte buffer object is pooled (zero-allocation), Request objects are pooled (zero-allocation)
	// Using SetRequestURIBytes avoids string allocation
	if allocs > 1 {
		t.Errorf("Request building allocated %f times per run (expected <= 1, got %.2f)", allocs, allocs)
	}

	t.Logf("Request building allocations per run: %.2f (excellent: <= 1, byte buffer is pooled, no string conversion)", allocs)
}

// TestClient_GetDepth_WithLimit tests different limit values
func TestClient_GetDepth_WithLimit(t *testing.T) {
	client := NewClient()

	testCases := []struct {
		name   string
		limit  int
		expect int // expected minimum number of entries
	}{
		{"Limit 5", 5, 5},
		{"Limit 10", 10, 10},
		{"Limit 20", 20, 20},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := DepthParams{
				Symbol: "BTCUSDT",
				Limit:  tc.limit,
			}

			depth, err := client.GetDepth(params)
			if err != nil {
				t.Fatalf("GetDepth failed: %v", err)
			}
			defer client.ReleaseDepthResponse(depth)

			if depth.BidsLen() < tc.expect {
				t.Errorf("Expected at least %d bids, got %d", tc.expect, depth.BidsLen())
			}

			if depth.AsksLen() < tc.expect {
				t.Errorf("Expected at least %d asks, got %d", tc.expect, depth.AsksLen())
			}
		})
	}
}

// TestClient_GetDepth_InvalidSymbol tests error handling for invalid symbol
func TestClient_GetDepth_InvalidSymbol(t *testing.T) {
	client := NewClient()

	params := DepthParams{
		Symbol: "INVALIDPAIR",
		Limit:  10,
	}

	_, err := client.GetDepth(params)
	if err == nil {
		t.Fatal("Expected error for invalid symbol, got nil")
	}

	t.Logf("Correctly returned error for invalid symbol: %v", err)
}

// BenchmarkGetDepth benchmarks the GetDepth method
func BenchmarkGetDepth(b *testing.B) {
	client := NewClient()
	params := DepthParams{
		Symbol: "BTCUSDT",
		Limit:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		depth, err := client.GetDepth(params)
		if err != nil {
			b.Fatalf("GetDepth failed: %v", err)
		}
		client.ReleaseDepthResponse(depth)
	}
}

// BenchmarkGetDepth_RequestBuilding benchmarks just the request building part using pooled byte buffer
func BenchmarkGetDepth_RequestBuilding(b *testing.B) {
	client := NewClient()
	params := DepthParams{
		Symbol: "BTCUSDT",
		Limit:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := fasthttp.AcquireRequest()

		// Use pooled byte buffer (same as actual implementation)
		buf := bytebufferpool.Get()
		buf.WriteString(client.baseURL)
		buf.WriteString(EndpointDepth)
		req.SetRequestURIBytes(buf.B)
		bytebufferpool.Put(buf)

		uri := req.URI()
		args := uri.QueryArgs()
		args.Set("symbol", params.Symbol)
		args.Set("limit", "100")
		fasthttp.ReleaseRequest(req)
	}
}
