package event

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/BullionBear/seq/core/model/common"
)

// ============================================================================
// Old (pre-P1-3) encoders, kept verbatim as the reference implementations
// for the differential tests until the Cursor migration is settled.
// ============================================================================

func oldEncodeDepthSnapshotLayout(buf []byte, symbolID, depthID int, timestamp uint64, asks, bids []common.PriceLevel) error {
	needed := DepthSnapshotHeaderSize + (len(asks)+len(bids))*PriceLevelSize
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(symbolID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(depthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], timestamp)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(asks)))
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(bids)))
	pos += 4
	for i := range asks {
		plBytes := (*[32]byte)(unsafe.Pointer(&asks[i]))[:PriceLevelSize]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	for i := range bids {
		plBytes := (*[32]byte)(unsafe.Pointer(&bids[i]))[:PriceLevelSize]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	return nil
}

func oldEncodeDepthUpdate(buf []byte, d DepthUpdate) error {
	if len(buf) < d.GetBufferLength() {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.SymbolID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.PreviousDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.DepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.CurrentDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(d.NextDepthID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], d.Timestamp)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(d.Asks)))
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(d.Bids)))
	pos += 4
	for i := range d.Asks {
		plBytes := (*[32]byte)(unsafe.Pointer(&d.Asks[i]))[:PriceLevelSize]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	for i := range d.Bids {
		plBytes := (*[32]byte)(unsafe.Pointer(&d.Bids[i]))[:PriceLevelSize]
		copy(buf[pos:], plBytes)
		pos += PriceLevelSize
	}
	return nil
}

func oldEncodeBalanceUpdate(buf []byte, b BalanceUpdate) error {
	if len(buf) < b.GetBufferLength() {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(b.AccountID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(b.WalletID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], b.UpdatedAt)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(b.Balances)))
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4
	for i := range b.Balances {
		balanceBytes := (*[32]byte)(unsafe.Pointer(&b.Balances[i]))[:BalanceSize]
		copy(buf[pos:], balanceBytes)
		pos += BalanceSize
	}
	return nil
}

func oldEncodeRespBalanceSnapshot(buf []byte, r RespBalanceSnapshot) error {
	if len(buf) < r.GetBufferLength() {
		return ErrBufferTooSmall
	}
	pos := 0
	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.AccountID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.WalletID))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], uint32(len(r.Balances)))
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0)
	pos += 4
	for i := range r.Balances {
		balanceBytes := (*[32]byte)(unsafe.Pointer(&r.Balances[i]))[:BalanceSize]
		copy(buf[pos:], balanceBytes)
		pos += BalanceSize
	}
	return nil
}

// ============================================================================
// Test helpers
// ============================================================================

func randomLevels(rng *rand.Rand, n int) []common.PriceLevel {
	if n == 0 {
		return nil
	}
	out := make([]common.PriceLevel, n)
	for i := range out {
		out[i] = common.PriceLevel{
			Price:        rng.NormFloat64() * 1e5,
			Quantity:     rng.Float64() * 100,
			PriceTick:    rng.Int(),
			QuantityTick: rng.Int(),
		}
	}
	return out
}

func randomBalances(rng *rand.Rand, n int) []common.Balance {
	if n == 0 {
		return nil
	}
	out := make([]common.Balance, n)
	for i := range out {
		out[i] = common.Balance{
			TokenID:   rng.Int(),
			Available: rng.Float64() * 1e6,
			Locked:    rng.Float64() * 1e3,
			Total:     rng.Float64() * 1e6,
		}
	}
	return out
}

// ============================================================================
// Differential tests: Cursor-based encoders vs. the old offset arithmetic
// ============================================================================

func TestDepthEncodersMatchOld(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for iter := 0; iter < 200; iter++ {
		nAsks, nBids := rng.Intn(20), rng.Intn(20)

		snap := DepthSnapshot{
			SymbolID:  rng.Int(),
			DepthID:   rng.Int(),
			Timestamp: rng.Uint64(),
			Asks:      randomLevels(rng, nAsks),
			Bids:      randomLevels(rng, nBids),
		}
		newBuf := make([]byte, snap.GetBufferLength())
		oldBuf := make([]byte, snap.GetBufferLength())
		if err := snap.Encode(newBuf); err != nil {
			t.Fatalf("DepthSnapshot.Encode: %v", err)
		}
		if err := oldEncodeDepthSnapshotLayout(oldBuf, snap.SymbolID, snap.DepthID, snap.Timestamp, snap.Asks, snap.Bids); err != nil {
			t.Fatalf("old encode: %v", err)
		}
		if !bytes.Equal(newBuf, oldBuf) {
			t.Fatal("DepthSnapshot encoding differs from old implementation")
		}

		upd := DepthUpdate{
			SymbolID:        rng.Int(),
			PreviousDepthID: rng.Int(),
			DepthID:         rng.Int(),
			CurrentDepthID:  rng.Int(),
			NextDepthID:     rng.Int(),
			Timestamp:       rng.Uint64(),
			Asks:            randomLevels(rng, nAsks),
			Bids:            randomLevels(rng, nBids),
		}
		newBuf = make([]byte, upd.GetBufferLength())
		oldBuf = make([]byte, upd.GetBufferLength())
		if err := upd.Encode(newBuf); err != nil {
			t.Fatalf("DepthUpdate.Encode: %v", err)
		}
		if err := oldEncodeDepthUpdate(oldBuf, upd); err != nil {
			t.Fatalf("old encode: %v", err)
		}
		if !bytes.Equal(newBuf, oldBuf) {
			t.Fatal("DepthUpdate encoding differs from old implementation")
		}

		resp := RespDepthSnapshot{
			SymbolID:  snap.SymbolID,
			DepthID:   snap.DepthID,
			Timestamp: snap.Timestamp,
			Asks:      snap.Asks,
			Bids:      snap.Bids,
		}
		newBuf = make([]byte, resp.GetBufferLength())
		oldBuf = make([]byte, resp.GetBufferLength())
		if err := resp.Encode(newBuf); err != nil {
			t.Fatalf("RespDepthSnapshot.Encode: %v", err)
		}
		if err := oldEncodeDepthSnapshotLayout(oldBuf, resp.SymbolID, resp.DepthID, resp.Timestamp, resp.Asks, resp.Bids); err != nil {
			t.Fatalf("old encode: %v", err)
		}
		if !bytes.Equal(newBuf, oldBuf) {
			t.Fatal("RespDepthSnapshot encoding differs from old implementation")
		}
	}
}

func TestBalanceEncodersMatchOld(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for iter := 0; iter < 200; iter++ {
		n := rng.Intn(10)

		upd := BalanceUpdate{
			AccountID: rng.Int(),
			WalletID:  rng.Int(),
			UpdatedAt: rng.Uint64(),
			Balances:  randomBalances(rng, n),
		}
		newBuf := make([]byte, upd.GetBufferLength())
		oldBuf := make([]byte, upd.GetBufferLength())
		if err := upd.Encode(newBuf); err != nil {
			t.Fatalf("BalanceUpdate.Encode: %v", err)
		}
		if err := oldEncodeBalanceUpdate(oldBuf, upd); err != nil {
			t.Fatalf("old encode: %v", err)
		}
		if !bytes.Equal(newBuf, oldBuf) {
			t.Fatal("BalanceUpdate encoding differs from old implementation")
		}

		resp := RespBalanceSnapshot{
			AccountID: rng.Int(),
			WalletID:  rng.Int(),
			Balances:  randomBalances(rng, n),
		}
		newBuf = make([]byte, resp.GetBufferLength())
		oldBuf = make([]byte, resp.GetBufferLength())
		if err := resp.Encode(newBuf); err != nil {
			t.Fatalf("RespBalanceSnapshot.Encode: %v", err)
		}
		if err := oldEncodeRespBalanceSnapshot(oldBuf, resp); err != nil {
			t.Fatalf("old encode: %v", err)
		}
		if !bytes.Equal(newBuf, oldBuf) {
			t.Fatal("RespBalanceSnapshot encoding differs from old implementation")
		}
	}
}

// ============================================================================
// View correctness
// ============================================================================

func TestDepthViewsMatchDecode(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	snap := DepthSnapshot{
		SymbolID:  42,
		DepthID:   1000,
		Timestamp: 1700000000,
		Asks:      randomLevels(rng, 5),
		Bids:      randomLevels(rng, 3),
	}
	buf := make([]byte, snap.GetBufferLength())
	if err := snap.Encode(buf); err != nil {
		t.Fatal(err)
	}
	v, err := NewDepthSnapshotView(buf)
	if err != nil {
		t.Fatal(err)
	}
	if v.SymbolID() != snap.SymbolID || v.DepthID() != snap.DepthID || v.Timestamp() != snap.Timestamp {
		t.Errorf("snapshot view header mismatch: %d/%d/%d", v.SymbolID(), v.DepthID(), v.Timestamp())
	}
	if v.NumAsks() != len(snap.Asks) || v.NumBids() != len(snap.Bids) {
		t.Fatalf("snapshot view counts mismatch: %d/%d", v.NumAsks(), v.NumBids())
	}
	for i := range snap.Asks {
		if v.Ask(i) != snap.Asks[i] {
			t.Errorf("Ask(%d) = %+v, want %+v", i, v.Ask(i), snap.Asks[i])
		}
	}
	for i := range snap.Bids {
		if v.Bid(i) != snap.Bids[i] {
			t.Errorf("Bid(%d) = %+v, want %+v", i, v.Bid(i), snap.Bids[i])
		}
	}

	upd := DepthUpdate{
		SymbolID:        7,
		PreviousDepthID: 99,
		DepthID:         100,
		CurrentDepthID:  105,
		NextDepthID:     106,
		Timestamp:       1700000001,
		Asks:            randomLevels(rng, 4),
		Bids:            randomLevels(rng, 6),
	}
	buf = make([]byte, upd.GetBufferLength())
	if err := upd.Encode(buf); err != nil {
		t.Fatal(err)
	}
	uv, err := NewDepthUpdateView(buf)
	if err != nil {
		t.Fatal(err)
	}
	if uv.SymbolID() != upd.SymbolID ||
		uv.PreviousDepthID() != upd.PreviousDepthID ||
		uv.DepthID() != upd.DepthID ||
		uv.CurrentDepthID() != upd.CurrentDepthID ||
		uv.NextDepthID() != upd.NextDepthID ||
		uv.Timestamp() != upd.Timestamp {
		t.Error("update view header mismatch")
	}
	for i := range upd.Asks {
		if uv.Ask(i) != upd.Asks[i] {
			t.Errorf("Ask(%d) mismatch", i)
		}
	}
	for i := range upd.Bids {
		if uv.Bid(i) != upd.Bids[i] {
			t.Errorf("Bid(%d) mismatch", i)
		}
	}

	// FromBytes must agree with the view it is built on.
	decoded, err := NewDepthUpdateFromBytes(buf)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SymbolID != upd.SymbolID || len(decoded.Asks) != len(upd.Asks) || len(decoded.Bids) != len(upd.Bids) {
		t.Error("NewDepthUpdateFromBytes mismatch")
	}
}

func TestDepthViewOutOfRangePanics(t *testing.T) {
	snap := DepthSnapshot{SymbolID: 1, Asks: randomLevels(rand.New(rand.NewSource(5)), 2)}
	buf := make([]byte, snap.GetBufferLength())
	if err := snap.Encode(buf); err != nil {
		t.Fatal(err)
	}
	v, err := NewDepthSnapshotView(buf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Error("Ask(NumAsks()) did not panic")
		}
	}()
	_ = v.Ask(v.NumAsks())
}

// TestDepthViewRejectsMalformed pins the constructor validation
// (header + n*PriceLevelSize invariant).
func TestDepthViewRejectsMalformed(t *testing.T) {
	if _, err := NewDepthSnapshotView(make([]byte, DepthSnapshotHeaderSize-1)); err != ErrBufferTooSmall {
		t.Errorf("short snapshot buffer: got %v, want ErrBufferTooSmall", err)
	}
	if _, err := NewDepthUpdateView(nil); err != ErrBufferTooSmall {
		t.Errorf("nil update buffer: got %v, want ErrBufferTooSmall", err)
	}

	// Header lies: claims levels the buffer does not carry.
	buf := make([]byte, DepthSnapshotHeaderSize)
	binary.LittleEndian.PutUint32(buf[snapAsksLenOff:], 3)
	if _, err := NewDepthSnapshotView(buf); err != ErrInvalidBuffer {
		t.Errorf("lying snapshot header: got %v, want ErrInvalidBuffer", err)
	}

	buf = make([]byte, DepthUpdateHeaderSize)
	binary.LittleEndian.PutUint32(buf[updAsksLenOff:], 0xFFFFFFFF) // overflow probe
	binary.LittleEndian.PutUint32(buf[updBidsLenOff:], 0xFFFFFFFF)
	if _, err := NewDepthUpdateView(buf); err != ErrInvalidBuffer {
		t.Errorf("lying update header: got %v, want ErrInvalidBuffer", err)
	}
}

// FuzzDepthViews feeds malformed buffers to the view constructors; a
// successfully constructed view must then survive full iteration.
func FuzzDepthViews(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, DepthSnapshotHeaderSize))
	f.Add(make([]byte, DepthUpdateHeaderSize))
	f.Add(make([]byte, DepthUpdateHeaderSize+PriceLevelSize))
	lying := make([]byte, DepthSnapshotHeaderSize)
	binary.LittleEndian.PutUint32(lying[snapAsksLenOff:], 0xFFFFFFFF)
	f.Add(lying)

	f.Fuzz(func(t *testing.T, buf []byte) {
		if v, err := NewDepthSnapshotView(buf); err == nil {
			sum := 0
			for i, n := 0, v.NumAsks(); i < n; i++ {
				sum += v.Ask(i).PriceTick
			}
			for i, n := 0, v.NumBids(); i < n; i++ {
				sum += v.Bid(i).PriceTick
			}
			_ = sum
		}
		if v, err := NewDepthUpdateView(buf); err == nil {
			sum := 0
			for i, n := 0, v.NumAsks(); i < n; i++ {
				sum += v.Ask(i).PriceTick
			}
			for i, n := 0, v.NumBids(); i < n; i++ {
				sum += v.Bid(i).PriceTick
			}
			_ = sum
		}
	})
}

// ============================================================================
// Allocation gates (P1-3): encode and full view iteration must not allocate.
// ============================================================================

func TestDepthEncodeAndViewZeroAllocs(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	upd := DepthUpdate{
		SymbolID:        1,
		PreviousDepthID: 2,
		DepthID:         3,
		CurrentDepthID:  4,
		NextDepthID:     5,
		Timestamp:       6,
		Asks:            randomLevels(rng, 10),
		Bids:            randomLevels(rng, 10),
	}
	buf := make([]byte, upd.GetBufferLength())

	if n := testing.AllocsPerRun(1000, func() {
		if err := upd.Encode(buf); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("DepthUpdate.Encode allocates %v per run, want 0", n)
	}

	if n := testing.AllocsPerRun(1000, func() {
		v, err := NewDepthUpdateView(buf)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0
		for i, n := 0, v.NumAsks(); i < n; i++ {
			sum += v.Ask(i).PriceTick
		}
		for i, n := 0, v.NumBids(); i < n; i++ {
			sum += v.Bid(i).PriceTick
		}
		if sum == 0 {
			t.Fatal("unexpected zero sum")
		}
	}); n != 0 {
		t.Errorf("DepthUpdateView full iteration allocates %v per run, want 0", n)
	}

	snap := DepthSnapshot{SymbolID: 1, DepthID: 2, Timestamp: 3, Asks: upd.Asks, Bids: upd.Bids}
	snapBuf := make([]byte, snap.GetBufferLength())
	if err := snap.Encode(snapBuf); err != nil {
		t.Fatal(err)
	}
	if n := testing.AllocsPerRun(1000, func() {
		v, err := NewDepthSnapshotView(snapBuf)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0
		for i, n := 0, v.NumAsks(); i < n; i++ {
			sum += v.Ask(i).PriceTick
		}
		for i, n := 0, v.NumBids(); i < n; i++ {
			sum += v.Bid(i).PriceTick
		}
		if sum == 0 {
			t.Fatal("unexpected zero sum")
		}
	}); n != 0 {
		t.Errorf("DepthSnapshotView full iteration allocates %v per run, want 0", n)
	}
}
