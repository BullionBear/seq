package bybit

import (
	"math"
	"testing"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

// newTestDataClient builds a client with one symbol and registered topics,
// mirroring what Connect() would set up.
func newTestDataClient(t testing.TB) (*BybitDataClient, *msgbus.MsgBus, *wsConnection) {
	t.Helper()

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]catalog.Symbol)
	symbols[1] = catalog.Symbol{
		ID:   1,
		Name: "BTCUSDT",
		Product: catalog.Product{
			ID:   1,
			Name: "Linear",
			Slug: "linear",
		},
		PricePrecision: 2,
		SizePrecision:  3,
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBybitDataClient(cat, eb)
	sym := symbols[1]
	client.registerTopic("orderbook.50.BTCUSDT", &sym)
	client.registerTopic("publicTrade.BTCUSDT", &sym)

	return client, eb, &wsConnection{client: client}
}

var bybitDeltaMsg = []byte(`{
	"topic": "orderbook.50.BTCUSDT",
	"type": "delta",
	"ts": 1687940967466,
	"data": {
		"s": "BTCUSDT",
		"b": [
			["30247.20", "30.028"],
			["30240.00", "0"],
			["30239.50", "1.250"],
			["30238.00", "0.500"],
			["30237.00", "2.000"]
		],
		"a": [
			["30248.70", "0"],
			["30249.30", "0.892"],
			["30250.00", "1.500"],
			["30251.00", "0.750"],
			["30252.00", "3.000"]
		],
		"u": 177400507,
		"seq": 66544703342
	},
	"cts": 1687940967464
}`)

var bybitSnapshotMsg = []byte(`{
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

var bybitTradeMsg = []byte(`{
	"topic": "publicTrade.BTCUSDT",
	"type": "snapshot",
	"ts": 1672304486868,
	"data": [
		{
			"T": 1672304486865,
			"s": "BTCUSDT",
			"S": "Sell",
			"v": "0.001",
			"p": "16578.50",
			"L": "PlusTick",
			"i": "20f43950-d8dd-5b31-9112-a178eb6023af",
			"BT": false
		}
	]
}`)

var bybitBuyTradeMsg = []byte(`{
	"topic": "publicTrade.BTCUSDT",
	"type": "snapshot",
	"ts": 1672304486868,
	"data": [
		{
			"T": 1672304486865,
			"s": "BTCUSDT",
			"S": "Buy",
			"v": "0.002",
			"p": "16579.00",
			"L": "PlusTick",
			"i": "20f43950-d8dd-5b31-9112-a178eb6023b0",
			"BT": false
		}
	]
}`)

// TestProcessTradeSides pins the Side mapping to the common.Side enum:
// "Sell" -> common.SideSell (2), "Buy" -> common.SideBuy (1). The previous
// implementation published 1 (SideBuy) for sells and 0 (SideUnknown) for buys.
func TestProcessTradeSides(t *testing.T) {
	client, eb, ws := newTestDataClient(t)

	decodeTick := func() event.Tick {
		var received msgbus.Event
		if !eb.Poll(func(e msgbus.Event) { received = e }) {
			t.Fatal("Expected trade event to be published")
		}
		if received.Ref.Topic != event.TopicEventTick {
			t.Fatalf("Expected TopicEventTick, got %v", received.Ref.Topic)
		}
		tick, err := event.NewTickFromBytes(eb.ReadBuffer(received.Ref.Index, received.Ref.Length))
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		return tick
	}

	client.processMessage(ws, bybitTradeMsg)
	tick := decodeTick()
	if tick.Side != common.SideSell {
		t.Errorf("Expected SideSell for S=Sell, got %v", tick.Side)
	}
	if tick.Price != 16578.50 || math.Abs(tick.Qty-0.001) > 1e-12 {
		t.Errorf("Trade fields mismatch: %+v", tick)
	}

	client.processMessage(ws, bybitBuyTradeMsg)
	tick = decodeTick()
	if tick.Side != common.SideBuy {
		t.Errorf("Expected SideBuy for S=Buy, got %v", tick.Side)
	}
}

func TestProcessOrderbookDeltaTicks(t *testing.T) {
	client, eb, ws := newTestDataClient(t)

	client.processMessage(ws, bybitDeltaMsg)

	var received msgbus.Event
	if !eb.Poll(func(e msgbus.Event) { received = e }) {
		t.Fatal("Expected depth update event to be published")
	}
	update, err := event.NewDepthUpdateFromBytes(eb.ReadBuffer(received.Ref.Index, received.Ref.Length))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(update.Bids) != 5 || len(update.Asks) != 5 {
		t.Fatalf("Expected 5x5 levels, got %dx%d", len(update.Bids), len(update.Asks))
	}
	// PricePrecision=2 -> tick = round(price * 100)
	if update.Bids[0].PriceTick != 3024720 {
		t.Errorf("Expected bid PriceTick 3024720, got %d", update.Bids[0].PriceTick)
	}
	// SizePrecision=3 -> tick = round(qty * 1000)
	if update.Bids[0].QuantityTick != 30028 {
		t.Errorf("Expected bid QuantityTick 30028, got %d", update.Bids[0].QuantityTick)
	}
}

// TestProcessMessageZeroAllocs gates the steady-state allocation budget of
// the WS parse+publish path (P2-2): after warm-up (scratch buffers at
// high-water, precision cache populated) processing must not allocate.
func TestProcessMessageZeroAllocs(t *testing.T) {
	client, eb, ws := newTestDataClient(t)
	drain := func(msgbus.Event) {}

	cases := []struct {
		name string
		msg  []byte
	}{
		{"snapshot", bybitSnapshotMsg},
		{"delta", bybitDeltaMsg},
		{"trade", bybitTradeMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Warm up scratch high-water marks and caches.
			for i := 0; i < 3; i++ {
				client.processMessage(ws, tc.msg)
				for eb.Poll(drain) {
				}
			}
			allocs := testing.AllocsPerRun(100, func() {
				client.processMessage(ws, tc.msg)
				for eb.Poll(drain) {
				}
			})
			if allocs != 0 {
				t.Errorf("processMessage(%s) allocated %.1f times per run, want 0", tc.name, allocs)
			}
		})
	}
}

func BenchmarkProcessOrderbookDelta(b *testing.B) {
	client, eb, ws := newTestDataClient(b)
	drain := func(msgbus.Event) {}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.processMessage(ws, bybitDeltaMsg)
		for eb.Poll(drain) {
		}
	}
}

func BenchmarkProcessPublicTrade(b *testing.B) {
	client, eb, ws := newTestDataClient(b)
	drain := func(msgbus.Event) {}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.processMessage(ws, bybitTradeMsg)
		for eb.Poll(drain) {
		}
	}
}
