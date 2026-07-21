package binance

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

// ============================================================================
// Unit Tests for parseFloat64
// ============================================================================

func TestParseFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected float64
	}{
		{"simple integer", []byte("123"), 123.0},
		{"simple decimal", []byte("123.456"), 123.456},
		{"quoted string", []byte(`"123.456"`), 123.456},
		{"negative number", []byte("-123.456"), -123.456},
		{"positive sign", []byte("+123.456"), 123.456},
		{"zero", []byte("0"), 0.0},
		{"zero decimal", []byte("0.0"), 0.0},
		{"small decimal", []byte("0.00000001"), 0.00000001},
		{"large number", []byte("99999.99999"), 99999.99999},
		{"binance price format", []byte("50000.00000000"), 50000.0},
		{"binance qty format", []byte("1.50000000"), 1.5},
		{"empty", []byte(""), 0.0},
		{"quoted empty", []byte(`""`), 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloat64(tt.input)
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("parseFloat64(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkParseFloat64(b *testing.B) {
	input := []byte("50000.12345678")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = parseFloat64(input)
	}
}

func BenchmarkParseFloat64Quoted(b *testing.B) {
	input := []byte(`"50000.12345678"`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = parseFloat64(input)
	}
}

// ============================================================================
// Unit Tests for URL Building
// ============================================================================

func TestBuildStreamURL(t *testing.T) {
	eb := msgbus.NewMsgBus()
	client := NewBinanceSpotDataClient(nil, eb)

	tests := []struct {
		name     string
		streams  []string
		expected string
	}{
		{
			name:     "empty streams",
			streams:  []string{},
			expected: BaseWsURL + "/ws",
		},
		{
			name:     "single stream",
			streams:  []string{"btcusdt@depth@100ms"},
			expected: BaseWsURL + "/ws/btcusdt@depth@100ms",
		},
		{
			name:     "multiple streams",
			streams:  []string{"btcusdt@depth@100ms", "ethusdt@trade"},
			expected: BaseWsURL + "/stream?streams=btcusdt@depth@100ms/ethusdt@trade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.buildStreamURL(tt.streams)
			if result != tt.expected {
				t.Errorf("buildStreamURL(%v) = %q, want %q", tt.streams, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Unit Tests for Message Processing
// ============================================================================

func TestProcessDepthUpdate(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject symbols
	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Register stream mapping
	client.streamToSymbol["btcusdt@depth@100ms"] = 1

	// Binance depth update message
	depthMsg := []byte(`{
		"e": "depthUpdate",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"U": 157,
		"u": 160,
		"b": [
			["50000.00", "1.5"],
			["49999.00", "2.0"]
		],
		"a": [
			["50001.00", "0.5"],
			["50002.00", "1.0"]
		]
	}`)

	// Process the message
	client.processDepthUpdate(1, depthMsg)

	// Verify event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected depth update event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventDepthUpdate {
		t.Errorf("Expected TopicEventDepthUpdate, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	depthUpdate, err := event.NewDepthUpdateFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if depthUpdate.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", depthUpdate.SymbolID)
	}

	if depthUpdate.DepthID != 157 {
		t.Errorf("Expected DepthID 157, got %d", depthUpdate.DepthID)
	}

	if depthUpdate.CurrentDepthID != 160 {
		t.Errorf("Expected CurrentDepthID 160, got %d", depthUpdate.CurrentDepthID)
	}

	if len(depthUpdate.Bids) != 2 {
		t.Fatalf("Expected 2 bids, got %d", len(depthUpdate.Bids))
	}

	if depthUpdate.Bids[0].Price != 50000.0 || depthUpdate.Bids[0].Quantity != 1.5 {
		t.Errorf("Bid[0] mismatch: got %+v", depthUpdate.Bids[0])
	}

	if len(depthUpdate.Asks) != 2 {
		t.Fatalf("Expected 2 asks, got %d", len(depthUpdate.Asks))
	}

	if depthUpdate.Asks[0].Price != 50001.0 || depthUpdate.Asks[0].Quantity != 0.5 {
		t.Errorf("Ask[0] mismatch: got %+v", depthUpdate.Asks[0])
	}
}

func TestStreamSuffix(t *testing.T) {
	tests := []struct {
		rate     PushRate
		levels   int
		expected string
	}{
		{PushRate100ms, 0, "depth@100ms"},
		{PushRate1s, 0, "depth@1000ms"},
		{PushRate100ms, 5, "depth5@100ms"},
		{PushRate100ms, 10, "depth10@100ms"},
		{PushRate1s, 20, "depth20@1000ms"},
		{PushRate100ms, 50, "depth@100ms"}, // non-partial levels → diff
	}
	for _, tt := range tests {
		got := tt.rate.StreamSuffix(tt.levels)
		if got != tt.expected {
			t.Errorf("StreamSuffix(%d,%d)=%q, want %q", tt.rate, tt.levels, got, tt.expected)
		}
	}
}

func TestIsPartialBookStream(t *testing.T) {
	tests := []struct {
		stream   string
		expected bool
	}{
		{"btcusdt@depth@100ms", false},
		{"btcusdt@depth@1000ms", false},
		{"btcusdt@depth5@100ms", true},
		{"btcusdt@depth10@100ms", true},
		{"ethusdt@depth20@1000ms", true},
		{"btcusdt@trade", false},
	}
	for _, tt := range tests {
		got := isPartialBookStream([]byte(tt.stream))
		if got != tt.expected {
			t.Errorf("isPartialBookStream(%q)=%v, want %v", tt.stream, got, tt.expected)
		}
	}
}

func TestProcessDepthSnapshot(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT", PricePrecision: 2, SizePrecision: 1}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@depth20@100ms"] = 1
	client.precisions[1] = newSymbolPrecision(2, 1)

	// Combined-stream partial book message (no "e" field)
	msg := []byte(`{
		"stream":"btcusdt@depth20@100ms",
		"data":{
			"lastUpdateId": 160,
			"bids": [
				["50000.00", "1.5"],
				["49999.00", "2.0"]
			],
			"asks": [
				["50001.00", "0.5"],
				["50002.00", "1.0"]
			]
		}
	}`)

	client.processMessage(msg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})
	if !ok {
		t.Fatal("Expected depth snapshot event to be published")
	}
	if receivedEvent.Ref.Topic != event.TopicEventDepthSnapshot {
		t.Errorf("Expected TopicEventDepthSnapshot, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	snapshot, err := event.NewDepthSnapshotFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if snapshot.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", snapshot.SymbolID)
	}
	if snapshot.DepthID != 160 {
		t.Errorf("Expected DepthID 160, got %d", snapshot.DepthID)
	}
	if len(snapshot.Bids) != 2 {
		t.Fatalf("Expected 2 bids, got %d", len(snapshot.Bids))
	}
	if snapshot.Bids[0].Price != 50000.0 || snapshot.Bids[0].Quantity != 1.5 {
		t.Errorf("Bid[0] mismatch: got %+v", snapshot.Bids[0])
	}
	if len(snapshot.Asks) != 2 {
		t.Fatalf("Expected 2 asks, got %d", len(snapshot.Asks))
	}
	if snapshot.Asks[0].Price != 50001.0 || snapshot.Asks[0].Quantity != 0.5 {
		t.Errorf("Ask[0] mismatch: got %+v", snapshot.Asks[0])
	}
}

func TestProcessKline(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@kline_1m"] = 1

	msg := []byte(`{
		"stream":"btcusdt@kline_1m",
		"data":{
			"e": "kline",
			"E": 1672515782136,
			"s": "BTCUSDT",
			"k": {
				"t": 1672515780000,
				"T": 1672515839999,
				"s": "BTCUSDT",
				"i": "1m",
				"f": 100,
				"L": 200,
				"o": "0.0010",
				"c": "0.0020",
				"h": "0.0025",
				"l": "0.0015",
				"v": "1000",
				"n": 100,
				"x": false,
				"q": "1.0000",
				"V": "500",
				"Q": "0.500",
				"B": "123456"
			}
		}
	}`)

	client.processMessage(msg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})
	if !ok {
		t.Fatal("Expected kline event to be published")
	}
	if receivedEvent.Ref.Topic != event.TopicEventKline {
		t.Errorf("Expected TopicEventKline, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	kline, err := event.NewKlineFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if kline.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", kline.SymbolID)
	}
	if kline.Interval != common.Interval1m {
		t.Errorf("Expected Interval1m, got %v", kline.Interval)
	}
	if math.Abs(kline.Open-0.0010) > 1e-12 || math.Abs(kline.High-0.0025) > 1e-12 ||
		math.Abs(kline.Low-0.0015) > 1e-12 || math.Abs(kline.Close-0.0020) > 1e-12 {
		t.Errorf("OHLC mismatch: %+v", kline)
	}
	if math.Abs(kline.Volume-1000) > 1e-12 || math.Abs(kline.QuoteVolume-1.0) > 1e-12 {
		t.Errorf("Volume mismatch: %+v", kline)
	}
	if kline.TradeCount != 100 {
		t.Errorf("Expected TradeCount 100, got %d", kline.TradeCount)
	}
	if kline.Closed {
		t.Error("Expected Closed=false")
	}
	if kline.StartTime != 1672515780000*1_000_000 {
		t.Errorf("Unexpected StartTime %d", kline.StartTime)
	}
}

func TestSubscribeKlineBuildStream(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.SubscribeKline(1, "1m")

	streams := client.buildStreamList()
	if len(streams) != 1 || streams[0] != "btcusdt@kline_1m" {
		t.Fatalf("Expected [btcusdt@kline_1m], got %v", streams)
	}
}

func TestProcessTrade(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	// Inject symbols
	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Register stream mapping
	client.streamToSymbol["btcusdt@trade"] = 1

	// Binance trade message - buyer is maker (sell)
	tradeMsg := []byte(`{
		"e": "trade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"t": 12345,
		"p": "50000.50",
		"q": "1.25",
		"T": 1672531200123,
		"m": true,
		"M": true
	}`)

	client.processTrade(1, tradeMsg)

	// Verify event was published
	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected trade event to be published")
	}

	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	tick, err := event.NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if tick.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", tick.SymbolID)
	}

	if tick.Price != 50000.50 {
		t.Errorf("Expected Price 50000.50, got %f", tick.Price)
	}

	if tick.Qty != 1.25 {
		t.Errorf("Expected Qty 1.25, got %f", tick.Qty)
	}

	if tick.Side != common.SideSell {
		t.Errorf("Expected SideSell (m=true), got %v", tick.Side)
	}

	// Test buy side (m=false)
	buyTradeMsg := []byte(`{
		"e": "trade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"t": 12346,
		"p": "50001.00",
		"q": "2.0",
		"T": 1672531200456,
		"m": false,
		"M": true
	}`)

	client.processTrade(1, buyTradeMsg)

	ok = eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected second trade event to be published")
	}

	buf = eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	tick, err = event.NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if tick.Side != common.SideBuy {
		t.Errorf("Expected SideBuy (m=false), got %v", tick.Side)
	}
}

func TestProcessMessage_CombinedStream(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@trade"] = 1

	// Combined stream format
	combinedMsg := []byte(`{
		"stream": "btcusdt@trade",
		"data": {
			"e": "trade",
			"E": 1672531200000,
			"s": "BTCUSDT",
			"t": 12345,
			"p": "50000.00",
			"q": "1.0",
			"T": 1672531200123,
			"m": false,
			"M": true
		}
	}`)

	client.processMessage(combinedMsg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected event from combined stream message")
	}

	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}
}

func TestProcessMessage_SingleStream(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@trade"] = 1
	client.symbolToID["BTCUSDT"] = 1

	// Single stream format (no "stream" wrapper)
	singleMsg := []byte(`{
		"e": "trade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"t": 12345,
		"p": "50000.00",
		"q": "1.0",
		"T": 1672531200123,
		"m": false,
		"M": true
	}`)

	client.processMessage(singleMsg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})

	if !ok {
		t.Fatal("Expected event from single stream message")
	}

	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}
}

func TestProcessAggTrade(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Binance aggTrade: trade ID is "a"; p/q/T/m match @trade.
	aggMsg := []byte(`{
		"e": "aggTrade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"a": 12345,
		"p": "50000.50",
		"q": "1.25",
		"f": 100,
		"l": 105,
		"T": 1672531200123,
		"m": true,
		"M": true
	}`)

	client.processTrade(1, aggMsg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})
	if !ok {
		t.Fatal("Expected aggTrade event to be published")
	}
	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	tick, err := event.NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if tick.SymbolID != 1 {
		t.Errorf("Expected SymbolID 1, got %d", tick.SymbolID)
	}
	if tick.Price != 50000.50 {
		t.Errorf("Expected Price 50000.50, got %f", tick.Price)
	}
	if tick.Qty != 1.25 {
		t.Errorf("Expected Qty 1.25, got %f", tick.Qty)
	}
	if tick.Side != common.SideSell {
		t.Errorf("Expected SideSell (m=true), got %v", tick.Side)
	}
	if tick.Timestamp != 1672531200123*1_000_000 {
		t.Errorf("Expected Timestamp %d, got %d", uint64(1672531200123*1_000_000), tick.Timestamp)
	}
}

func TestProcessMessage_CombinedStreamAggTrade(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@aggTrade"] = 1

	combinedMsg := []byte(`{
		"stream": "btcusdt@aggTrade",
		"data": {
			"e": "aggTrade",
			"E": 1672531200000,
			"s": "BTCUSDT",
			"a": 12345,
			"p": "50000.00",
			"q": "1.0",
			"f": 100,
			"l": 105,
			"T": 1672531200123,
			"m": false,
			"M": true
		}
	}`)

	client.processMessage(combinedMsg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})
	if !ok {
		t.Fatal("Expected event from combined aggTrade stream message")
	}
	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}

	buf := eb.ReadBuffer(receivedEvent.Ref.Index, receivedEvent.Ref.Length)
	tick, err := event.NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if tick.Side != common.SideBuy {
		t.Errorf("Expected SideBuy (m=false), got %v", tick.Side)
	}
}

func TestProcessMessage_SingleStreamAggTrade(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.symbolToID["BTCUSDT"] = 1

	singleMsg := []byte(`{
		"e": "aggTrade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"a": 12345,
		"p": "50000.00",
		"q": "1.0",
		"f": 100,
		"l": 105,
		"T": 1672531200123,
		"m": false,
		"M": true
	}`)

	client.processMessage(singleMsg)

	var receivedEvent msgbus.Event
	ok := eb.Poll(func(e msgbus.Event) {
		receivedEvent = e
	})
	if !ok {
		t.Fatal("Expected event from single aggTrade stream message")
	}
	if receivedEvent.Ref.Topic != event.TopicEventTick {
		t.Errorf("Expected TopicEventTick, got %v", receivedEvent.Ref.Topic)
	}
}

func TestIsTradeStream(t *testing.T) {
	cases := []struct {
		stream string
		want   bool
	}{
		{"btcusdt@trade", true},
		{"btcusdt@aggTrade", true},
		{"btcusdt@depth@100ms", false},
		{"btcusdt@depth5@100ms", false},
		{"btcusdt@kline_1m", false},
	}
	for _, tc := range cases {
		if got := isTradeStream([]byte(tc.stream)); got != tc.want {
			t.Errorf("isTradeStream(%q)=%v, want %v", tc.stream, got, tc.want)
		}
	}
}

// ============================================================================
// Benchmark Tests for Message Processing
// ============================================================================

func BenchmarkProcessDepthUpdate(b *testing.B) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@depth@100ms"] = 1

	// Realistic depth update with 10 levels
	depthMsg := []byte(`{
		"e": "depthUpdate",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"U": 157,
		"u": 160,
		"b": [
			["50000.00", "1.5"],
			["49999.00", "2.0"],
			["49998.00", "3.0"],
			["49997.00", "4.0"],
			["49996.00", "5.0"]
		],
		"a": [
			["50001.00", "0.5"],
			["50002.00", "1.0"],
			["50003.00", "1.5"],
			["50004.00", "2.0"],
			["50005.00", "2.5"]
		]
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.processDepthUpdate(1, depthMsg)
		// Drain the event bus
		eb.Poll(func(e msgbus.Event) {})
	}
}

func BenchmarkProcessTrade(b *testing.B) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@trade"] = 1

	tradeMsg := []byte(`{
		"e": "trade",
		"E": 1672531200000,
		"s": "BTCUSDT",
		"t": 12345,
		"p": "50000.12345678",
		"q": "1.23456789",
		"T": 1672531200123,
		"m": true,
		"M": true
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.processTrade(1, tradeMsg)
		// Drain the event bus
		eb.Poll(func(e msgbus.Event) {})
	}
}

func BenchmarkProcessMessage_CombinedStream(b *testing.B) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@trade"] = 1

	combinedMsg := []byte(`{"stream":"btcusdt@trade","data":{"e":"trade","E":1672531200000,"s":"BTCUSDT","t":12345,"p":"50000.00","q":"1.0","T":1672531200123,"m":false,"M":true}}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.processMessage(combinedMsg)
		eb.Poll(func(e msgbus.Event) {})
	}
}

// ============================================================================
// Unit Tests for Subscription Management
// ============================================================================

func TestSubscribeDepthUpdate(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	symbols[2] = catalog.Symbol{ID: 2, Name: "ETHUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	client.SubscribeDepthUpdate(1, 0, 100)
	client.SubscribeDepthUpdate(2, 0, 1000)

	if len(client.depthSubs) != 2 {
		t.Errorf("Expected 2 depth subscriptions, got %d", len(client.depthSubs))
	}

	if _, ok := client.depthSubs[1]; !ok {
		t.Error("Expected subscription for symbolID 1")
	}

	if _, ok := client.depthSubs[2]; !ok {
		t.Error("Expected subscription for symbolID 2")
	}
}

func TestSubscribeTrade(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	symbols[2] = catalog.Symbol{ID: 2, Name: "ETHUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	client.SubscribeTrade(1)
	client.SubscribeAggTrade(2)

	if len(client.tradeSubs) != 1 {
		t.Errorf("Expected 1 trade subscription, got %d", len(client.tradeSubs))
	}
	if _, ok := client.tradeSubs[1]; !ok {
		t.Fatal("Expected trade subscription for symbolID 1")
	}

	if len(client.aggTradeSubs) != 1 {
		t.Errorf("Expected 1 aggTrade subscription, got %d", len(client.aggTradeSubs))
	}
	if _, ok := client.aggTradeSubs[2]; !ok {
		t.Fatal("Expected aggTrade subscription for symbolID 2")
	}
}

func TestBuildStreamList(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	symbols[2] = catalog.Symbol{ID: 2, Name: "ETHUSDT"}
	symbols[3] = catalog.Symbol{ID: 3, Name: "BNBUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	client.SubscribeDepthUpdate(1, 0, 100)
	client.SubscribeTrade(2)
	client.SubscribeAggTrade(3)

	streams := client.buildStreamList()

	if len(streams) != 3 {
		t.Fatalf("Expected 3 streams, got %d", len(streams))
	}

	// Check stream names (order may vary)
	hasDepth := false
	hasTrade := false
	hasAggTrade := false
	for _, s := range streams {
		if s == "btcusdt@depth@100ms" {
			hasDepth = true
		}
		if s == "ethusdt@trade" {
			hasTrade = true
		}
		if s == "bnbusdt@aggTrade" {
			hasAggTrade = true
		}
	}

	if !hasDepth {
		t.Error("Expected btcusdt@depth@100ms stream")
	}
	if !hasTrade {
		t.Error("Expected ethusdt@trade stream")
	}
	if !hasAggTrade {
		t.Error("Expected bnbusdt@aggTrade stream")
	}

	// Verify stream to symbol mapping
	if client.streamToSymbol["btcusdt@depth@100ms"] != 1 {
		t.Error("Stream mapping for depth not set correctly")
	}
	if client.streamToSymbol["bnbusdt@aggTrade"] != 3 {
		t.Error("Stream mapping for aggTrade not set correctly")
	}
	if client.streamToSymbol["ethusdt@trade"] != 2 {
		t.Error("Stream mapping for trade not set correctly")
	}
}

func TestBuildStreamListPartialBook(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.SubscribeDepthUpdate(1, 20, 100)

	streams := client.buildStreamList()
	if len(streams) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(streams))
	}
	if streams[0] != "btcusdt@depth20@100ms" {
		t.Errorf("Expected btcusdt@depth20@100ms, got %q", streams[0])
	}
	if client.streamToSymbol["btcusdt@depth20@100ms"] != 1 {
		t.Error("Stream mapping for partial depth not set correctly")
	}
	if !client.depthSubs[1].IsPartialBook() {
		t.Error("Expected partial-book subscription options")
	}
}

// ============================================================================
// Concurrency Tests
// ============================================================================

func TestConcurrentSubscriptions(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	for i := 1; i <= 100; i++ {
		symbols[i] = catalog.Symbol{ID: i, Name: fmt.Sprintf("SYM%d", i)}
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			client.SubscribeDepthUpdate(id, 0, 100)
		}(i)
		go func(id int) {
			defer wg.Done()
			client.SubscribeTrade(id)
		}(i)
	}
	wg.Wait()

	if len(client.depthSubs) != 100 {
		t.Errorf("Expected 100 depth subscriptions, got %d", len(client.depthSubs))
	}
	if len(client.tradeSubs) != 100 {
		t.Errorf("Expected 100 trade subscriptions, got %d", len(client.tradeSubs))
	}
}

func TestConcurrentMessageProcessing(t *testing.T) {
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	client.streamToSymbol["btcusdt@trade"] = 1
	client.symbolToID["BTCUSDT"] = 1

	tradeMsg := []byte(`{"e":"trade","E":1672531200000,"s":"BTCUSDT","t":12345,"p":"50000.00","q":"1.0","T":1672531200123,"m":false,"M":true}`)

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.processMessage(tradeMsg)
		}()
	}
	wg.Wait()

	// Count events (some may have been dropped due to buffer limits)
	count := 0
	for {
		ok := eb.Poll(func(e msgbus.Event) {
			count++
		})
		if !ok {
			break
		}
	}

	if count == 0 {
		t.Error("Expected some events to be published")
	}
	t.Logf("Processed %d events from concurrent message processing", count)
}

// ============================================================================
// Integration Test (connects to real Binance WebSocket)
// ============================================================================

// TestIntegration_RealConnection tests real connection to Binance WebSocket
// Run with: go test -v -run TestIntegration_RealConnection -tags=integration
func TestIntegration_RealConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Subscribe to trade stream
	client.SubscribeTrade(1)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Wait for some messages
	t.Log("Connected to Binance WebSocket, waiting for trades...")

	receivedCount := 0
	timeout := time.After(15 * time.Second)

loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			ok := eb.Poll(func(e msgbus.Event) {
				if e.Ref.Topic == event.TopicEventTick {
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					tick, err := event.NewTickFromBytes(buf)
					if err != nil {
						t.Fatalf("decode failed: %v", err)
					}
					t.Logf("Received trade: Price=%.2f, Qty=%.4f, Side=%v",
						tick.Price, tick.Qty, tick.Side)
					receivedCount++
				}
			})
			if !ok {
				time.Sleep(10 * time.Millisecond)
			}
			if receivedCount >= 5 {
				break loop
			}
		}
	}

	if receivedCount == 0 {
		t.Error("Did not receive any trades")
	} else {
		t.Logf("Received %d trades total", receivedCount)
	}
}

// TestIntegration_DepthUpdates tests depth update stream
func TestIntegration_DepthUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Subscribe to depth stream
	client.SubscribeDepthUpdate(1, 0, 100)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Binance WebSocket, waiting for depth updates...")

	receivedCount := 0
	timeout := time.After(15 * time.Second)

loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			ok := eb.Poll(func(e msgbus.Event) {
				if e.Ref.Topic == event.TopicEventDepthUpdate {
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					depth, err := event.NewDepthUpdateFromBytes(buf)
					if err != nil {
						t.Fatalf("decode failed: %v", err)
					}
					t.Logf("Received depth update: DepthID=%d, Bids=%d, Asks=%d",
						depth.DepthID, len(depth.Bids), len(depth.Asks))
					receivedCount++
				}
			})
			if !ok {
				time.Sleep(10 * time.Millisecond)
			}
			if receivedCount >= 5 {
				break loop
			}
		}
	}

	if receivedCount == 0 {
		t.Error("Did not receive any depth updates")
	} else {
		t.Logf("Received %d depth updates total", receivedCount)
	}
}

// TestIntegration_MultipleStreams tests subscribing to multiple streams
func TestIntegration_MultipleStreams(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{ID: 1, Name: "BTCUSDT"}
	symbols[2] = catalog.Symbol{ID: 2, Name: "ETHUSDT"}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)

	// Subscribe to multiple streams
	client.SubscribeTrade(1)
	client.SubscribeTrade(2)
	client.SubscribeDepthUpdate(1, 0, 100)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	t.Log("Connected to Binance WebSocket with multiple streams...")

	btcTrades := 0
	ethTrades := 0
	depthUpdates := 0
	timeout := time.After(15 * time.Second)

loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			ok := eb.Poll(func(e msgbus.Event) {
				switch e.Ref.Topic {
				case event.TopicEventTick:
					buf := eb.ReadBuffer(e.Ref.Index, e.Ref.Length)
					tick, err := event.NewTickFromBytes(buf)
					if err != nil {
						t.Fatalf("decode failed: %v", err)
					}
					if tick.SymbolID == 1 {
						btcTrades++
					} else if tick.SymbolID == 2 {
						ethTrades++
					}
				case event.TopicEventDepthUpdate:
					depthUpdates++
				}
			})
			if !ok {
				time.Sleep(10 * time.Millisecond)
			}
			if btcTrades >= 2 && ethTrades >= 2 && depthUpdates >= 2 {
				break loop
			}
		}
	}

	t.Logf("Received: BTC trades=%d, ETH trades=%d, Depth updates=%d",
		btcTrades, ethTrades, depthUpdates)

	if btcTrades == 0 {
		t.Error("Did not receive any BTC trades")
	}
	if ethTrades == 0 {
		t.Error("Did not receive any ETH trades")
	}
	if depthUpdates == 0 {
		t.Error("Did not receive any depth updates")
	}
}

// Helper to set private fields (reusing from http_test.go)
func setPrivateFieldDataClient(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}
