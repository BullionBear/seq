package binance

import (
	"testing"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/msgbus"
)

// newAllocTestClient builds a client with one symbol registered the way
// Connect() would (stream map, symbol map, precision cache populated).
func newAllocTestClient(t testing.TB) (*BinanceSpotDataClient, *msgbus.MsgBus) {
	t.Helper()

	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}

	symbols := make(map[int]cpanel.Symbol)
	symbols[1] = cpanel.Symbol{ID: 1, Name: "BTCUSDT", PricePrecision: 2, SizePrecision: 8}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceSpotDataClient(cat, eb)
	sym := symbols[1]
	client.registerStream("btcusdt@depth@100ms", &sym)
	client.registerStream("btcusdt@trade", &sym)

	return client, eb
}

var binanceCombinedDepthMsg = []byte(`{"stream":"btcusdt@depth@100ms","data":{"e":"depthUpdate","E":1672531200000,"s":"BTCUSDT","U":157,"u":160,"b":[["50000.00","1.5"],["49999.00","2.0"],["49998.00","3.0"],["49997.00","4.0"],["49996.00","5.0"]],"a":[["50001.00","0.5"],["50002.00","1.0"],["50003.00","1.5"],["50004.00","2.0"],["50005.00","2.5"]]}}`)

var binanceCombinedTradeMsg = []byte(`{"stream":"btcusdt@trade","data":{"e":"trade","E":1672531200000,"s":"BTCUSDT","t":12345,"p":"50000.00","q":"1.0","T":1672531200123,"m":false,"M":true}}`)

var binanceSingleTradeMsg = []byte(`{"e":"trade","E":1672531200000,"s":"BTCUSDT","t":12345,"p":"50000.00","q":"1.0","T":1672531200123,"m":true,"M":true}`)

// TestProcessMessageZeroAllocs gates the steady-state allocation budget of
// the WS parse+publish path (P2-2): after warm-up (scratch buffers at
// high-water, precision cache populated) processing must not allocate.
func TestProcessMessageZeroAllocs(t *testing.T) {
	client, eb := newAllocTestClient(t)
	drain := func(msgbus.Event) {}

	cases := []struct {
		name string
		msg  []byte
	}{
		{"combined depth", binanceCombinedDepthMsg},
		{"combined trade", binanceCombinedTradeMsg},
		{"single-stream trade", binanceSingleTradeMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Warm up scratch high-water marks and caches.
			for i := 0; i < 3; i++ {
				client.processMessage(tc.msg)
				for eb.Poll(drain) {
				}
			}
			allocs := testing.AllocsPerRun(100, func() {
				client.processMessage(tc.msg)
				for eb.Poll(drain) {
				}
			})
			if allocs != 0 {
				t.Errorf("processMessage(%s) allocated %.1f times per run, want 0", tc.name, allocs)
			}
		})
	}
}
