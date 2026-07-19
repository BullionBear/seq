package bybit

import (
	"math"
	"testing"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

func TestParseBybitKlinesReversesOrder(t *testing.T) {
	// Newest first as Bybit returns.
	list := []byte(`[
		["1670608800000","17071","17073","17027","17055.5","268611","15.74462667"],
		["1670605200000","17071.5","17071.5","17061","17071","4177","0.24469757"],
		["1670601600000","17086.5","17088","16978","17071.5","6356","0.37288112"]
	]`)
	bars, err := parseBybitKlines(list)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 3 {
		t.Fatalf("got %d bars", len(bars))
	}
	// After reverse: oldest first
	if bars[0].StartTime != 1670601600000*1_000_000 {
		t.Errorf("oldest start=%d", bars[0].StartTime)
	}
	if bars[2].StartTime != 1670608800000*1_000_000 {
		t.Errorf("newest start=%d", bars[2].StartTime)
	}
	if math.Abs(bars[0].Open-17086.5) > 1e-9 || !bars[0].Closed {
		t.Errorf("bar0=%+v", bars[0])
	}
}

func TestRespHistoricalKlinePublishBybitShape(t *testing.T) {
	eb := msgbus.NewMsgBus()
	bars, err := parseBybitKlines([]byte(`[["1000","1","2","0.5","1.5","10","15"]]`))
	if err != nil {
		t.Fatal(err)
	}
	out := event.RespHistoricalKline{SymbolID: 2, Interval: common.Interval5m, Bars: bars}
	ref, buf, ok := eb.Allocate(event.TopicEventRespHistoricalKline, uint64(out.GetBufferLength()))
	if !ok {
		t.Fatal("allocate failed")
	}
	if err := out.Encode(buf); err != nil {
		t.Fatal(err)
	}
	eb.Publish(ref)

	var received msgbus.Event
	if !eb.Poll(func(e msgbus.Event) { received = e }) {
		t.Fatal("expected event")
	}
	decoded, err := event.NewRespHistoricalKlineFromBytes(eb.ReadBuffer(received.Ref.Index, received.Ref.Length))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Interval != common.Interval5m || len(decoded.Bars) != 1 {
		t.Fatalf("decoded=%+v", decoded)
	}
}
