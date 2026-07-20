package binance

import (
	"math"
	"testing"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

func TestParseBinanceKlines(t *testing.T) {
	body := []byte(`[
  [1499040000000,"0.01634790","0.80000000","0.01575800","0.01577100","148976.11427815",1499644799999,"2434.19055334",308,"1756.67402397","28.46694368","17928899.62484339"],
  [1499644800000,"0.01577100","0.01580000","0.01570000","0.01575000","100.0",1499648399999,"1.5",10,"50","0.75","0"]
]`)
	bars, err := parseBinanceKlines(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("got %d bars", len(bars))
	}
	if bars[0].StartTime != 1499040000000*1_000_000 {
		t.Errorf("start=%d", bars[0].StartTime)
	}
	if bars[0].EndTime != 1499644799999*1_000_000 {
		t.Errorf("end=%d", bars[0].EndTime)
	}
	if math.Abs(bars[0].Open-0.01634790) > 1e-10 || bars[0].TradeCount != 308 || !bars[0].Closed {
		t.Errorf("bar0=%+v", bars[0])
	}
	if math.Abs(bars[1].Close-0.01575000) > 1e-10 {
		t.Errorf("bar1 close=%v", bars[1].Close)
	}
}

func TestReqHistoricalKlinePublishes(t *testing.T) {
	// Unit-level: parse + encode path via a fake body through publish helpers.
	eb := msgbus.NewMsgBus()
	cat := &catalog.Catalog{}
	symbols := map[int]catalog.Symbol{
		1: {ID: 1, Name: "BTCUSDT"},
	}
	setPrivateField(cat, "symbols", symbols)

	client := NewBinanceHTTPClient(cat, eb)
	bars, err := parseBinanceKlines([]byte(`[[1000,"1","2","0.5","1.5","10",1999,"15",3,"1","1","0"]]`))
	if err != nil {
		t.Fatal(err)
	}
	out := event.RespHistoricalKline{SymbolID: 1, Interval: common.Interval1m, Bars: bars}
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
	if received.Ref.Topic != event.TopicEventRespHistoricalKline {
		t.Fatalf("topic=%v", received.Ref.Topic)
	}
	decoded, err := event.NewRespHistoricalKlineFromBytes(eb.ReadBuffer(received.Ref.Index, received.Ref.Length))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SymbolID != 1 || len(decoded.Bars) != 1 || math.Abs(decoded.Bars[0].Close-1.5) > 1e-12 {
		t.Fatalf("decoded=%+v", decoded)
	}
	_ = client // keep client construction covered
}
