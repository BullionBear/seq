package event

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/BullionBear/seq/core/model/common"
)

func TestAppendJSON_RoundTripTick(t *testing.T) {
	in := Tick{SymbolID: 1, Timestamp: 99, Side: common.SideBuy, Price: 1.5, Qty: 2.25}
	raw := in.AppendJSON(nil)
	var out Tick
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestAppendJSON_RoundTripOrderNew(t *testing.T) {
	in := OrderNew{
		AccountID: 1, ClientOrderID: 2, OrderID: 3, SymbolID: 4,
		Side: common.SideSell, OrderType: common.OrderTypeLimit,
		TimeInForce: common.TimeInForceGTC, Quantity: 1, Price: 2,
		ExecutedQty: 0.5, CreatedAt: 10, UpdatedAt: 11,
	}
	raw := in.AppendJSON(nil)
	var out OrderNew
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestAppendJSON_RoundTripOrderError(t *testing.T) {
	in := OrderError{
		ClientOrderID: 1, OrderID: 2, AccountID: 3, ErrorCode: 4,
		Msg: "quote \"and\" \\ slash\n",
	}
	raw := in.AppendJSON(nil)
	var out OrderError
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if out != in {
		t.Fatalf("got %+v want %+v\n%s", out, in, raw)
	}
}

func TestAppendJSON_RoundTripDepthSnapshot(t *testing.T) {
	in := DepthSnapshot{
		SymbolID: 1, DepthID: 2, Timestamp: 3,
		Asks: []common.PriceLevel{{Price: 1, Quantity: 2, PriceTick: 3, QuantityTick: 4}},
		Bids: []common.PriceLevel{},
	}
	raw := in.AppendJSON(nil)
	var out DepthSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestAppendJSON_MatchesEncodingJSON_Tick(t *testing.T) {
	in := Tick{SymbolID: 7, Timestamp: 8, Side: common.SideBuy, Price: 9.25, Qty: 0.5}
	want, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	got := in.AppendJSON(nil)
	if string(got) != string(want) {
		t.Fatalf("AppendJSON drifted from encoding/json:\n got %s\nwant %s", got, want)
	}
}

func TestAppendEventJSON_ZeroAllocTick(t *testing.T) {
	tick := Tick{SymbolID: 1, Price: 2, Qty: 3, Timestamp: 4}
	buf := make([]byte, tick.GetBufferLength())
	if err := tick.Encode(buf); err != nil {
		t.Fatal(err)
	}
	dst := make([]byte, 0, 256)
	allocs := testing.AllocsPerRun(1000, func() {
		dst = AppendEventJSON(dst[:0], TopicEventTick, buf)
	})
	if allocs != 0 {
		t.Fatalf("allocs/op = %v, want 0", allocs)
	}
}
