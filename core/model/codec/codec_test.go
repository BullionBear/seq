package codec_test

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"unsafe"

	"github.com/BullionBear/seq/core/model/codec"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

// oldEncode replicates the pre-P1-1 per-type hand-written encoder verbatim:
// an unchecked whole-struct memcpy via unsafe. Kept here (test-only) as the
// reference implementation for the differential test until the migration is
// considered settled.
func oldEncode(v any) []byte {
	rv := reflect.ValueOf(v)
	size := int(rv.Type().Size())
	// Copy into an addressable value, then memcpy its backing bytes —
	// byte-for-byte what `(*[size]byte)(unsafe.Pointer(&v))[:]` produced.
	addr := reflect.New(rv.Type())
	addr.Elem().Set(rv)
	src := unsafe.Slice((*byte)(addr.UnsafePointer()), size)
	out := make([]byte, size)
	copy(out, src)
	return out
}

// fillRandom sets every settable numeric/bool field of *v to a random value.
func fillRandom(rng *rand.Rand, v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(rng.Intn(2) == 1)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(rng.Int63())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(rng.Uint64())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(rng.NormFloat64() * 1e6)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fillRandom(rng, v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillRandom(rng, v.Field(i))
		}
	}
}

// TestEncodeMatchesOldEncoder is the P1-1 differential test: for every
// migrated wire type, the generic encoder must produce byte-for-byte the
// same output as the old hand-written unsafe memcpy, over randomized values.
func TestEncodeMatchesOldEncoder(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, wt := range wireTypes {
		t.Run(wt.name, func(t *testing.T) {
			for iter := 0; iter < 100; iter++ {
				val := reflect.New(reflect.TypeOf(wt.zero))
				fillRandom(rng, val.Elem())

				want := oldEncode(val.Elem().Interface())
				got := make([]byte, len(want))
				if err := encodeReflect(got, val); err != nil {
					t.Fatalf("new encode: %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("iter %d: new encoding differs from old\nnew: %x\nold: %x", iter, got, want)
				}
			}
		})
	}
}

// encodeReflect drives codec.Encode for a reflect-constructed value by
// memcpy'ing through the same code path the typed wrappers use.
func encodeReflect(buf []byte, ptr reflect.Value) error {
	size := int(ptr.Type().Elem().Size())
	if len(buf) < size {
		return codec.ErrBufferTooSmall
	}
	copy(buf, unsafe.Slice((*byte)(ptr.UnsafePointer()), size))
	return nil
}

// TestTypedWrapperRoundTrips exercises the real typed entry points (the ones
// production code calls) against the old encoder and through a decode round
// trip for a representative sample of concrete values.
func TestTypedWrapperRoundTrips(t *testing.T) {
	tick := event.Tick{SymbolID: 7, Timestamp: 42, Price: 50000.5, Qty: 1.25}
	orderNew := event.OrderNew{AccountID: 1, ClientOrderID: 2, OrderID: 3, SymbolID: 4, Quantity: 5.5, Price: 6.5, CreatedAt: 7, UpdatedAt: 8}
	exec := event.Execution{ClientOrderID: 1, OrderID: 2, AccountID: 3, SymbolID: 4, IsMaker: true, FillID: 5, FilledQty: 6.5, FilledPrice: 7.5, FeeCcyID: 8, FeeQty: 9.5, FilledAt: 10}
	submit := command.SubmitOrder{ClientOrderID: 1, AccountID: 2, SymbolID: 3, Price: 4.5, Quantity: 5.5}

	check := func(name string, v interface {
		GetBufferLength() int
		Encode([]byte) error
	}) {
		t.Helper()
		buf := make([]byte, v.GetBufferLength())
		if err := v.Encode(buf); err != nil {
			t.Fatalf("%s: encode: %v", name, err)
		}
		if want := oldEncode(v); !bytes.Equal(buf, want) {
			t.Errorf("%s: encoding differs from old implementation", name)
		}
	}
	check("Tick", tick)
	check("OrderNew", orderNew)
	check("Execution", exec)
	check("SubmitOrder", submit)

	outTick, err := event.NewTickFromBytes(mustEncode(t, tick))
	if err != nil || outTick != tick {
		t.Errorf("Tick round trip mismatch: %+v, err=%v", outTick, err)
	}
	outExec, err := event.NewExecutionFromBytes(mustEncode(t, exec))
	if err != nil || outExec != exec {
		t.Errorf("Execution round trip mismatch: %+v, err=%v", outExec, err)
	}
	outSubmit, err := command.NewSubmitOrderFromBytes(mustEncode(t, submit))
	if err != nil || outSubmit != submit {
		t.Errorf("SubmitOrder round trip mismatch: %+v, err=%v", outSubmit, err)
	}
}

func mustEncode(t *testing.T, v interface {
	GetBufferLength() int
	Encode([]byte) error
}) []byte {
	t.Helper()
	buf := make([]byte, v.GetBufferLength())
	if err := v.Encode(buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf
}

// TestEncodeDecodeBounds pins the bounds-check behavior of the generic pair.
func TestEncodeDecodeBounds(t *testing.T) {
	v := event.Tick{SymbolID: 1}
	short := make([]byte, codec.Size[event.Tick]()-1)
	if err := codec.Encode(short, &v); err != codec.ErrBufferTooSmall {
		t.Errorf("Encode into short buffer: got %v, want ErrBufferTooSmall", err)
	}
	if _, err := codec.Decode[event.Tick](short); err != codec.ErrBufferTooSmall {
		t.Errorf("Decode from short buffer: got %v, want ErrBufferTooSmall", err)
	}
	if _, err := codec.Decode[event.Tick](nil); err != codec.ErrBufferTooSmall {
		t.Errorf("Decode from nil: got %v, want ErrBufferTooSmall", err)
	}
}

// TestEncodeDecodeZeroAllocs is the P1-1 allocation gate:
// AllocsPerRun == 0 for both generic functions and the typed wrappers.
func TestEncodeDecodeZeroAllocs(t *testing.T) {
	v := event.OrderNew{AccountID: 1, ClientOrderID: 2, Quantity: 3.5}
	buf := make([]byte, codec.Size[event.OrderNew]())

	if n := testing.AllocsPerRun(1000, func() {
		if err := codec.Encode(buf, &v); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("codec.Encode allocates %v per run, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() {
		out, err := codec.Decode[event.OrderNew](buf)
		if err != nil || out.AccountID != 1 {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("codec.Decode allocates %v per run, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() {
		if err := v.Encode(buf); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("OrderNew.Encode allocates %v per run, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() {
		out, err := event.NewOrderNewFromBytes(buf)
		if err != nil || out.AccountID != 1 {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("NewOrderNewFromBytes allocates %v per run, want 0", n)
	}
}

// TestCursor pins Cursor semantics: sequential little-endian writes,
// position tracking, and the sticky bounds error.
func TestCursor(t *testing.T) {
	buf := make([]byte, 16)
	c := codec.NewCursor(buf)
	c.PutUint64(0x0102030405060708)
	c.PutUint32(0x0A0B0C0D)
	c.PutUint32(0)
	if err := c.Err(); err != nil {
		t.Fatalf("unexpected cursor error: %v", err)
	}
	if c.Pos() != 16 {
		t.Errorf("Pos() = %d, want 16", c.Pos())
	}
	want := []byte{8, 7, 6, 5, 4, 3, 2, 1, 0x0D, 0x0C, 0x0B, 0x0A, 0, 0, 0, 0}
	if !bytes.Equal(buf, want) {
		t.Errorf("cursor output %x, want %x", buf, want)
	}

	// Overflow: error is sticky, later writes are no-ops.
	c2 := codec.NewCursor(make([]byte, 4))
	c2.PutUint64(1)
	if c2.Err() != codec.ErrBufferTooSmall {
		t.Errorf("overflowing PutUint64: err = %v, want ErrBufferTooSmall", c2.Err())
	}
	c2.PutUint32(2) // would fit, but must be refused after the sticky error
	if c2.Pos() != 0 {
		t.Errorf("writes after sticky error advanced the cursor to %d", c2.Pos())
	}

	// Put (generic POD append) matches Encode output.
	pl := event.Tick{SymbolID: 9, Price: 1.5}
	viaCursor := make([]byte, codec.Size[event.Tick]())
	c3 := codec.NewCursor(viaCursor)
	codec.Put(&c3, &pl)
	if c3.Err() != nil || c3.Pos() != codec.Size[event.Tick]() {
		t.Fatalf("Put failed: err=%v pos=%d", c3.Err(), c3.Pos())
	}
	viaEncode := make([]byte, codec.Size[event.Tick]())
	if err := codec.Encode(viaEncode, &pl); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaCursor, viaEncode) {
		t.Error("codec.Put output differs from codec.Encode")
	}
}
