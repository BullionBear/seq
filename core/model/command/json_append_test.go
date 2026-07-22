package command

import (
	"encoding/json"
	"testing"

	"github.com/BullionBear/seq/core/model/common"
)

func TestAppendJSON_RoundTripSubmitOrder(t *testing.T) {
	in := SubmitOrder{
		ClientOrderID: 1, AccountID: 2, SymbolID: 3,
		Side: common.SideBuy, OrderType: common.OrderTypeLimit,
		TimeInForce: common.TimeInForceIOC, Price: 10.5, Quantity: 2,
	}
	raw := in.AppendJSON(nil)
	var out SubmitOrder
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestAppendCommandJSON_ZeroAllocSubmit(t *testing.T) {
	in := SubmitOrder{ClientOrderID: 1, AccountID: 2, SymbolID: 3, Price: 1, Quantity: 2}
	buf := make([]byte, in.GetBufferLength())
	if err := in.Encode(buf); err != nil {
		t.Fatal(err)
	}
	dst := make([]byte, 0, 256)
	allocs := testing.AllocsPerRun(1000, func() {
		dst = AppendCommandJSON(dst[:0], CommandTypeOrderSubmit, buf)
	})
	if allocs != 0 {
		t.Fatalf("allocs/op = %v, want 0", allocs)
	}
}
