package command

import "testing"

// FuzzDecoders feeds arbitrary bytes to every command decoder. The P0-3
// contract: decoders must return an error for truncated input and must never
// panic or read out of bounds.
func FuzzDecoders(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add(make([]byte, 7))
	f.Add(make([]byte, 16))
	f.Add(make([]byte, 128))

	f.Fuzz(func(t *testing.T, buf []byte) {
		_, _ = NewRiskCheckFromBytes(buf)
		_, _ = NewSubmitOrderFromBytes(buf)
		_, _ = NewCancelOrderFromBytes(buf)
		_, _ = NewCancelAllFromBytes(buf)
		_, _ = NewReqDepthSnapshotFromBytes(buf)
		_, _ = NewQryBalanceSnapshotFromBytes(buf)
	})
}

// TestRoundTripSubmitOrder pins encode/decode symmetry after the copy-out change.
func TestRoundTripSubmitOrder(t *testing.T) {
	in := SubmitOrder{ClientOrderID: 7, AccountID: 1, SymbolID: 42, Price: 50000, Quantity: 1.5}
	buf := make([]byte, in.GetBufferLength())
	if err := in.Encode(buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := NewSubmitOrderFromBytes(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch: %+v != %+v", out, in)
	}
}

// TestDecodersRejectShortBuffers pins the bounds checks.
func TestDecodersRejectShortBuffers(t *testing.T) {
	short := make([]byte, 4)
	if _, err := NewSubmitOrderFromBytes(short); err == nil {
		t.Error("NewSubmitOrderFromBytes accepted a short buffer")
	}
	if _, err := NewRiskCheckFromBytes(nil); err == nil {
		t.Error("NewRiskCheckFromBytes accepted nil")
	}
	if _, err := NewReqDepthSnapshotFromBytes(short); err == nil {
		t.Error("NewReqDepthSnapshotFromBytes accepted a short buffer")
	}
}
