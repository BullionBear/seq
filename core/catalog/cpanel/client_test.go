package cpanel

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

// mockSymbolResponse creates a mock symbol response for testing
func mockSymbolResponse() []Symbol {
	return []Symbol{
		{
			ID:   1,
			Name: "BTCUSDT",
			Exchange: Exchange{
				ID:   1,
				Name: "Binance",
				Slug: "binance",
			},
			Product: Product{
				ID:   1,
				Name: "SPOT",
				Slug: "spot",
			},
			BaseToken: Token{
				ID:   1,
				Name: "BTC",
				Slug: "bitcoin",
			},
			QuoteToken: Token{
				ID:   3,
				Name: "USDT",
				Slug: "tether",
			},
			PricePrecision:  2,
			SizePrecision:   5,
			UniversalTicker: "BINANCE_SPOT_BTCUSDT",
		},
		{
			ID:   2,
			Name: "ETHUSDT",
			Exchange: Exchange{
				ID:   1,
				Name: "Binance",
				Slug: "binance",
			},
			Product: Product{
				ID:   1,
				Name: "SPOT",
				Slug: "spot",
			},
			BaseToken: Token{
				ID:   2,
				Name: "ETH",
				Slug: "ethereum",
			},
			QuoteToken: Token{
				ID:   3,
				Name: "USDT",
				Slug: "tether",
			},
			PricePrecision:  2,
			SizePrecision:   4,
			UniversalTicker: "BINANCE_SPOT_ETHUSDT",
		},
	}
}

// createTestServer creates a fasthttp test server with a handler
func createTestServer(handler fasthttp.RequestHandler) (*fasthttp.Client, *fasthttputil.InmemoryListener) {
	ln := fasthttputil.NewInmemoryListener()
	server := &fasthttp.Server{
		Handler: handler,
	}
	go server.Serve(ln)
	return &fasthttp.Client{
		Dial: func(addr string) (net.Conn, error) {
			return ln.Dial()
		},
	}, ln
}

func TestLynxLinkageClient_GetSymbol_Success(t *testing.T) {
	mockResp := mockSymbolResponse()
	jsonData, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		if string(ctx.Path()) != "/api/v1/symbol" {
			ctx.Error("Not Found", fasthttp.StatusNotFound)
			return
		}
		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Write(jsonData)
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}
	result, err := client.GetSymbol(context.Background(), params)
	if err != nil {
		t.Fatalf("GetSymbol failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 symbols, got %d", len(result))
	}

	if result[0].Name != "BTCUSDT" {
		t.Errorf("Expected first symbol name 'BTCUSDT', got '%s'", result[0].Name)
	}

	if result[0].Exchange.Name != "Binance" {
		t.Errorf("Expected exchange name 'Binance', got '%s'", result[0].Exchange.Name)
	}

	if result[1].Name != "ETHUSDT" {
		t.Errorf("Expected second symbol name 'ETHUSDT', got '%s'", result[1].Name)
	}
}

func TestLynxLinkageClient_GetSymbol_WithQueryParams(t *testing.T) {
	mockResp := []Symbol{
		{
			ID:   1,
			Name: "BTCUSDT",
			Exchange: Exchange{
				ID:   1,
				Name: "Binance",
				Slug: "binance",
			},
			Product: Product{
				ID:   1,
				Name: "SPOT",
				Slug: "spot",
			},
			BaseToken: Token{
				ID:   1,
				Name: "BTC",
				Slug: "bitcoin",
			},
			QuoteToken: Token{
				ID:   3,
				Name: "USDT",
				Slug: "tether",
			},
			PricePrecision:  2,
			SizePrecision:   5,
			UniversalTicker: "BINANCE_SPOT_BTCUSDT",
		},
	}
	jsonData, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		if string(ctx.Path()) != "/api/v1/symbol" {
			ctx.Error("Not Found", fasthttp.StatusNotFound)
			return
		}

		// Verify query parameters
		args := ctx.QueryArgs()
		if string(args.Peek("exchange_slug")) != "binance" {
			ctx.Error("Invalid exchange_slug", fasthttp.StatusBadRequest)
			return
		}
		if string(args.Peek("product_id")) != "1" {
			ctx.Error("Invalid product_id", fasthttp.StatusBadRequest)
			return
		}

		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Write(jsonData)
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	exchangeSlug := "binance"
	productID := 1
	params := SymbolParams{
		ExchangeSlug: &exchangeSlug,
		ProductID:    &productID,
	}

	result, err := client.GetSymbol(context.Background(), params)
	if err != nil {
		t.Fatalf("GetSymbol failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 symbol, got %d", len(result))
	}
}

func TestLynxLinkageClient_GetSymbol_AllQueryParams(t *testing.T) {
	mockResp := mockSymbolResponse()
	jsonData, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		args := ctx.QueryArgs()
		// Verify all query parameters are present
		if len(args.Peek("exchange_id")) == 0 {
			ctx.Error("Missing exchange_id", fasthttp.StatusBadRequest)
			return
		}
		if len(args.Peek("exchange_slug")) == 0 {
			ctx.Error("Missing exchange_slug", fasthttp.StatusBadRequest)
			return
		}
		if len(args.Peek("product_id")) == 0 {
			ctx.Error("Missing product_id", fasthttp.StatusBadRequest)
			return
		}
		if len(args.Peek("product_slug")) == 0 {
			ctx.Error("Missing product_slug", fasthttp.StatusBadRequest)
			return
		}
		if len(args.Peek("base_token_id")) == 0 {
			ctx.Error("Missing base_token_id", fasthttp.StatusBadRequest)
			return
		}
		if len(args.Peek("quote_token_id")) == 0 {
			ctx.Error("Missing quote_token_id", fasthttp.StatusBadRequest)
			return
		}

		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Write(jsonData)
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	exchangeID := 1
	exchangeSlug := "binance"
	productID := 1
	productSlug := "spot"
	baseTokenID := 1
	quoteTokenID := 3

	params := SymbolParams{
		ExchangeID:   &exchangeID,
		ExchangeSlug: &exchangeSlug,
		ProductID:    &productID,
		ProductSlug:  &productSlug,
		BaseTokenID:  &baseTokenID,
		QuoteTokenID: &quoteTokenID,
	}

	result, err := client.GetSymbol(context.Background(), params)
	if err != nil {
		t.Fatalf("GetSymbol failed: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("GetSymbol returned empty response")
	}
}

func TestLynxLinkageClient_GetSymbol_Non200Status(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.WriteString("Internal Server Error")
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}
	_, err := client.GetSymbol(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error for non-200 status, got nil")
	}

	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func TestLynxLinkageClient_GetSymbol_InvalidJSON(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("invalid json {")
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}
	_, err := client.GetSymbol(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestLynxLinkageClient_GetSymbol_ContextTimeout(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("[]")
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	params := SymbolParams{}
	_, err := client.GetSymbol(ctx, params)
	if err == nil {
		t.Fatal("Expected error for context timeout, got nil")
	}
}

func TestLynxLinkageClient_GetSymbol_ContextDeadlineExceeded(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("[]")
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	// Create a context that's already expired
	ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancel()

	params := SymbolParams{}
	_, err := client.GetSymbol(ctx, params)
	if err == nil {
		t.Fatal("Expected error for expired context, got nil")
	}

	if err.Error() != "context deadline exceeded" {
		t.Errorf("Expected 'context deadline exceeded' error, got: %v", err)
	}
}

func TestLynxLinkageClient_GetSymbol_EmptyResponse(t *testing.T) {
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("[]")
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}
	result, err := client.GetSymbol(context.Background(), params)
	if err != nil {
		t.Fatalf("GetSymbol failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty response, got %d symbols", len(result))
	}
}

func TestLynxLinkageClient_GetSymbol_ResponseStructure(t *testing.T) {
	mockResp := mockSymbolResponse()
	jsonData, err := json.Marshal(mockResp)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Write(jsonData)
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}
	result, err := client.GetSymbol(context.Background(), params)
	if err != nil {
		t.Fatalf("GetSymbol failed: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("GetSymbol returned empty response")
	}

	symbol := result[0]

	// Verify all fields are populated correctly
	if symbol.ID != 1 {
		t.Errorf("Expected ID 1, got %d", symbol.ID)
	}
	if symbol.Name != "BTCUSDT" {
		t.Errorf("Expected Name 'BTCUSDT', got '%s'", symbol.Name)
	}
	if symbol.Exchange.ID != 1 {
		t.Errorf("Expected Exchange.ID 1, got %d", symbol.Exchange.ID)
	}
	if symbol.Exchange.Name != "Binance" {
		t.Errorf("Expected Exchange.Name 'Binance', got '%s'", symbol.Exchange.Name)
	}
	if symbol.Exchange.Slug != "binance" {
		t.Errorf("Expected Exchange.Slug 'binance', got '%s'", symbol.Exchange.Slug)
	}
	if symbol.Product.ID != 1 {
		t.Errorf("Expected Product.ID 1, got %d", symbol.Product.ID)
	}
	if symbol.Product.Name != "SPOT" {
		t.Errorf("Expected Product.Name 'SPOT', got '%s'", symbol.Product.Name)
	}
	if symbol.BaseToken.ID != 1 {
		t.Errorf("Expected BaseToken.ID 1, got %d", symbol.BaseToken.ID)
	}
	if symbol.BaseToken.Name != "BTC" {
		t.Errorf("Expected BaseToken.Name 'BTC', got '%s'", symbol.BaseToken.Name)
	}
	if symbol.QuoteToken.ID != 3 {
		t.Errorf("Expected QuoteToken.ID 3, got %d", symbol.QuoteToken.ID)
	}
	if symbol.QuoteToken.Name != "USDT" {
		t.Errorf("Expected QuoteToken.Name 'USDT', got '%s'", symbol.QuoteToken.Name)
	}
	if symbol.PricePrecision != 2 {
		t.Errorf("Expected PricePrecision 2, got %d", symbol.PricePrecision)
	}
	if symbol.SizePrecision != 5 {
		t.Errorf("Expected SizePrecision 5, got %d", symbol.SizePrecision)
	}
	if symbol.UniversalTicker != "BINANCE_SPOT_BTCUSDT" {
		t.Errorf("Expected UniversalTicker 'BINANCE_SPOT_BTCUSDT', got '%s'", symbol.UniversalTicker)
	}
}

func BenchmarkLynxLinkageClient_GetSymbol(b *testing.B) {
	mockResp := mockSymbolResponse()
	jsonData, err := json.Marshal(mockResp)
	if err != nil {
		b.Fatalf("Failed to marshal mock response: %v", err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Write(jsonData)
	}

	testClient, ln := createTestServer(handler)
	defer ln.Close()

	client := &CpanelClient{
		baseURL: "http://test",
		client:  testClient,
	}

	params := SymbolParams{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.GetSymbol(context.Background(), params)
		if err != nil {
			b.Fatalf("GetSymbol failed: %v", err)
		}
	}
}
