package event

import (
	"testing"
)

// FuzzDecoders feeds arbitrary bytes to every event decoder. The P0-3
// contract: decoders must return an error for truncated or malformed input
// and must never panic or read out of bounds.
func FuzzDecoders(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add(make([]byte, 7))
	f.Add(make([]byte, 24))
	f.Add(make([]byte, 32))
	f.Add(make([]byte, 128))
	// Depth header claiming more levels than the buffer holds.
	f.Add([]byte{
		1, 0, 0, 0, 0, 0, 0, 0, // SymbolID
		2, 0, 0, 0, 0, 0, 0, 0, // DepthID
		3, 0, 0, 0, 0, 0, 0, 0, // Timestamp
		0xFF, 0xFF, 0xFF, 0xFF, // AsksLen (huge)
		0xFF, 0xFF, 0xFF, 0xFF, // BidsLen (huge)
	})
	// String header claiming a longer Msg than the buffer holds.
	f.Add([]byte{
		1, 0, 0, 0, 0, 0, 0, 0,
		2, 0, 0, 0, 0, 0, 0, 0,
		3, 0, 0, 0, 0, 0, 0, 0,
		0xFF, 0xFF, 0xFF, 0x7F,
	})

	f.Fuzz(func(t *testing.T, buf []byte) {
		// Fixed-size decoders.
		_, _ = NewTickFromBytes(buf)
		_, _ = NewTimeEventFromBytes(buf)
		_, _ = NewReadyEventFromBytes(buf)
		_, _ = NewStopEventFromBytes(buf)
		_, _ = NewFinishedEventFromBytes(buf)
		_, _ = NewAbnormalEventFromBytes(buf)
		_, _ = NewOrderNewFromBytes(buf)
		_, _ = NewOrderAcceptedFromBytes(buf)
		_, _ = NewOrderPartiallyFilledFromBytes(buf)
		_, _ = NewOrderFilledFromBytes(buf)
		_, _ = NewOrderCanceledFromBytes(buf)
		_, _ = NewExecutionFromBytes(buf)

		// Variable-size decoders (length-validated views).
		_, _ = NewDepthSnapshotFromBytes(buf)
		_, _ = NewDepthUpdateFromBytes(buf)
		_, _ = NewRespDepthSnapshotFromBytes(buf)
		_, _ = NewRespBalanceSnapshotFromBytes(buf)
		_, _ = NewBalanceUpdateFromBytes(buf)

		// String-carrying decoders.
		_, _ = NewOrderUnknownStatusFromBytes(buf)
		_, _ = NewOrderErrorFromBytes(buf)
		_, _ = NewOrderRejectedFromBytes(buf)
		_, _ = NewOrderRiskInvalidFromBytes(buf)
	})
}

// TestDecodersRejectShortBuffers pins the bounds-check behavior for every
// decoder without relying on the fuzzer.
func TestDecodersRejectShortBuffers(t *testing.T) {
	short := make([]byte, 4)
	if _, err := NewTickFromBytes(short); err == nil {
		t.Error("NewTickFromBytes accepted a short buffer")
	}
	if _, err := NewOrderNewFromBytes(short); err == nil {
		t.Error("NewOrderNewFromBytes accepted a short buffer")
	}
	if _, err := NewDepthSnapshotFromBytes(short); err == nil {
		t.Error("NewDepthSnapshotFromBytes accepted a short buffer")
	}
	if _, err := NewRespBalanceSnapshotFromBytes(short); err == nil {
		t.Error("NewRespBalanceSnapshotFromBytes accepted a short buffer")
	}
	if _, err := NewOrderErrorFromBytes(short); err == nil {
		t.Error("NewOrderErrorFromBytes accepted a short buffer")
	}
	if _, err := NewTimeEventFromBytes(nil); err == nil {
		t.Error("NewTimeEventFromBytes accepted nil")
	}
}

// TestDepthDecoderRejectsLyingHeader pins the structural validation: a header
// that declares more levels than the buffer carries must be rejected.
func TestDepthDecoderRejectsLyingHeader(t *testing.T) {
	buf := make([]byte, DepthSnapshotHeaderSize)
	buf[24] = 200 // AsksLen = 200, but zero levels present
	if _, err := NewDepthSnapshotFromBytes(buf); err != ErrInvalidBuffer {
		t.Errorf("expected ErrInvalidBuffer, got %v", err)
	}
}

// TestRoundTripTick pins encode/decode symmetry after the copy-out change.
func TestRoundTripTick(t *testing.T) {
	in := Tick{SymbolID: 42, Price: 123.45, Qty: 6.7, Timestamp: 999}
	buf := make([]byte, in.GetBufferLength())
	if err := in.Encode(buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch: %+v != %+v", out, in)
	}
}

// TestDecodeFromUnalignedBuffer verifies the copy-out decoders work from any
// byte offset (the msglog read path hands decoders unaligned buffers).
func TestDecodeFromUnalignedBuffer(t *testing.T) {
	in := Tick{SymbolID: 1, Price: 2.5}
	backing := make([]byte, in.GetBufferLength()+1)
	buf := backing[1:] // deliberately misaligned
	if err := in.Encode(buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := NewTickFromBytes(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch from unaligned buffer: %+v != %+v", out, in)
	}
}
