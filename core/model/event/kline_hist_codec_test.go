package event

import (
	"math"
	"testing"

	"github.com/BullionBear/seq/core/model/common"
)

func TestRespHistoricalKlineRoundTrip(t *testing.T) {
	in := RespHistoricalKline{
		SymbolID: 7,
		Interval: common.Interval1m,
		Bars: []KlineBar{
			{
				StartTime: 1_000_000_000, EndTime: 2_000_000_000, Timestamp: 1_000_000_000,
				Open: 10, High: 12, Low: 9, Close: 11, Volume: 100, QuoteVolume: 1100,
				TradeCount: 5, Closed: true,
			},
			{
				StartTime: 2_000_000_000, EndTime: 3_000_000_000, Timestamp: 2_000_000_000,
				Open: 11, High: 13, Low: 10, Close: 12, Volume: 200, QuoteVolume: 2400,
				TradeCount: 8, Closed: true,
			},
		},
	}
	buf := make([]byte, in.GetBufferLength())
	if err := in.Encode(buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := NewRespHistoricalKlineFromBytes(buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.SymbolID != in.SymbolID || out.Interval != in.Interval {
		t.Fatalf("header mismatch: %+v", out)
	}
	if len(out.Bars) != 2 {
		t.Fatalf("bars=%d, want 2", len(out.Bars))
	}
	if math.Abs(out.Bars[1].Close-12) > 1e-12 || out.Bars[1].TradeCount != 8 {
		t.Fatalf("bar[1] mismatch: %+v", out.Bars[1])
	}
}

func TestRespHistoricalKlineEmpty(t *testing.T) {
	in := RespHistoricalKline{SymbolID: 1, Interval: common.Interval5m}
	buf := make([]byte, in.GetBufferLength())
	if err := in.Encode(buf); err != nil {
		t.Fatal(err)
	}
	out, err := NewRespHistoricalKlineFromBytes(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Bars) != 0 {
		t.Fatalf("want 0 bars, got %d", len(out.Bars))
	}
}
