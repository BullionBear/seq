package mem

import (
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func TestCircularByteArena_BasicOperations(t *testing.T) {
	arena := NewCircularByteArena(1024)

	if arena.Claimed() != 0 || arena.Released() != 0 {
		t.Errorf("Expected zero cursors, got claimed=%d released=%d", arena.Claimed(), arena.Released())
	}
	if arena.Capacity() != 1024 {
		t.Errorf("Expected capacity 1024, got %d", arena.Capacity())
	}

	res, ok := arena.TryReserve(100)
	if !ok {
		t.Fatal("Expected reservation to succeed")
	}
	if res.Offset != 0 {
		t.Errorf("Expected offset 0, got %d", res.Offset)
	}
	// 100 rounds up to 104 for 8-byte alignment.
	if res.End-res.Start != 104 {
		t.Errorf("Expected claimed range of 104 bytes, got %d", res.End-res.Start)
	}

	slice := arena.GetSlice(res.Offset, 100)
	testData := []byte("hello world")
	copy(slice, testData)

	readSlice := arena.ReadSlice(res.Offset, uint64(len(testData)))
	if string(readSlice) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(readSlice))
	}

	arena.Release(res)
	if arena.Released() != res.End {
		t.Errorf("Expected released=%d, got %d", res.End, arena.Released())
	}
}

func TestCircularByteArena_BoundaryPadding(t *testing.T) {
	arena := NewCircularByteArena(96)

	res1, ok := arena.TryReserve(80)
	if !ok || res1.Offset != 0 {
		t.Fatalf("Expected first reservation at offset 0, got %+v ok=%v", res1, ok)
	}
	arena.Release(res1)

	// 80 used, 16 left at the tail. A 32-byte reservation cannot straddle the
	// end: the 16-byte tail is claimed as padding and payload starts at 0.
	res2, ok := arena.TryReserve(32)
	if !ok {
		t.Fatal("Expected second reservation to succeed")
	}
	if res2.Offset != 0 {
		t.Errorf("Expected payload to wrap to offset 0, got %d", res2.Offset)
	}
	if res2.End-res2.Start != 16+32 {
		t.Errorf("Expected range to include 16 bytes of padding, got %d bytes", res2.End-res2.Start)
	}
}

func TestCircularByteArena_BoundedByConsumer(t *testing.T) {
	arena := NewCircularByteArena(128)

	res1, ok := arena.TryReserve(64)
	if !ok {
		t.Fatal("first reservation should succeed")
	}
	res2, ok := arena.TryReserve(64)
	if !ok {
		t.Fatal("second reservation should succeed")
	}

	// Arena is full: no overwrite is possible, reservation must fail.
	if _, ok := arena.TryReserve(8); ok {
		t.Fatal("reservation on a full arena must fail")
	}

	// Out-of-order release: releasing res2 alone must not free the frontier.
	arena.Release(res2)
	if _, ok := arena.TryReserve(8); ok {
		t.Fatal("frontier is still held by res1; reservation must fail")
	}

	// Releasing res1 merges the parked res2 range and frees everything.
	arena.Release(res1)
	if arena.Released() != res2.End {
		t.Errorf("Expected released frontier %d, got %d", res2.End, arena.Released())
	}
	if _, ok := arena.TryReserve(64); !ok {
		t.Fatal("reservation should succeed after both releases")
	}
}

// TestCircularByteArena_AlignmentAcrossWraps asserts the P0-3 invariant:
// every payload offset is 8-byte aligned, including across wrap boundaries.
func TestCircularByteArena_AlignmentAcrossWraps(t *testing.T) {
	arena := NewCircularByteArena(256)

	sizes := []uint64{1, 3, 7, 8, 9, 15, 24, 31, 40, 63, 100}
	for i := 0; i < 500; i++ {
		size := sizes[i%len(sizes)]
		res, ok := arena.TryReserve(size)
		if !ok {
			t.Fatalf("iteration %d: reservation of %d bytes failed unexpectedly", i, size)
		}
		if res.Offset%8 != 0 {
			t.Fatalf("iteration %d: offset %d is not 8-byte aligned", i, res.Offset)
		}
		if res.Start%8 != 0 || res.End%8 != 0 {
			t.Fatalf("iteration %d: range [%d,%d) not 8-aligned", i, res.Start, res.End)
		}
		arena.Release(res)
	}
}

// TestCircularByteArena_ChecksumStress is the P0-2 verification: N producers
// on a small arena (forcing wraps every few hundred writes) write
// checksum-carrying payloads; the consumer validates every payload before
// releasing. Zero mismatches expected.
//
// The default iteration count is CI-friendly; set SEQ_STRESS_ITERS for the
// full run (e.g. SEQ_STRESS_ITERS=100000000).
func TestCircularByteArena_ChecksumStress(t *testing.T) {
	iters := 200_000
	if v := os.Getenv("SEQ_STRESS_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			iters = n
		}
	}
	if testing.Short() {
		iters = 50_000
	}

	const numProducers = 8
	const payloadSize = 40 // header(8) + body(24) + checksum(8)
	arena := NewCircularByteArena(4096)

	type published struct {
		res Reservation
	}
	ch := make(chan published, 1024)

	perProducer := iters / numProducers
	var wg sync.WaitGroup
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				var res Reservation
				for {
					var ok bool
					res, ok = arena.TryReserve(payloadSize)
					if ok {
						break
					}
					runtime.Gosched()
				}
				buf := arena.GetSlice(res.Offset, payloadSize)
				seq := uint64(id)<<32 | uint64(i)
				binary.LittleEndian.PutUint64(buf[0:], seq)
				var sum uint64
				for j := 8; j < payloadSize-8; j++ {
					buf[j] = byte(seq + uint64(j))
					sum += uint64(buf[j])
				}
				binary.LittleEndian.PutUint64(buf[payloadSize-8:], sum)
				ch <- published{res: res}
			}
		}(p)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	count := 0
	for pub := range ch {
		buf := arena.ReadSlice(pub.res.Offset, payloadSize)
		var sum uint64
		for j := 8; j < payloadSize-8; j++ {
			sum += uint64(buf[j])
		}
		if got := binary.LittleEndian.Uint64(buf[payloadSize-8:]); got != sum {
			t.Fatalf("checksum mismatch after %d events: got %d want %d", count, got, sum)
		}
		arena.Release(pub.res)
		count++
	}
	if count != perProducer*numProducers {
		t.Errorf("Expected %d events, got %d", perProducer*numProducers, count)
	}
}

// TestCircularByteArena_WrapWhileConsumerHoldsSlice exercises the wrap path:
// a producer tries to wrap onto a region the consumer still holds. The
// reservation must fail until the consumer releases.
func TestCircularByteArena_WrapWhileConsumerHoldsSlice(t *testing.T) {
	arena := NewCircularByteArena(64)

	resA, ok := arena.TryReserve(40)
	if !ok {
		t.Fatal("resA should succeed")
	}
	bufA := arena.GetSlice(resA.Offset, 40)
	for i := range bufA {
		bufA[i] = 0xAA
	}

	// Fill the rest of the tail.
	resB, ok := arena.TryReserve(24)
	if !ok {
		t.Fatal("resB should succeed")
	}
	arena.Release(resB)

	// A wrap onto resA's region must be refused while the consumer holds it.
	if _, ok := arena.TryReserve(40); ok {
		t.Fatal("wrap onto an unreleased region must fail")
	}
	for i := range bufA {
		if bufA[i] != 0xAA {
			t.Fatalf("consumer-held data was corrupted at byte %d", i)
		}
	}

	arena.Release(resA)
	if _, ok := arena.TryReserve(40); !ok {
		t.Fatal("reservation should succeed after release")
	}
}

func TestCircularByteArena_Reset(t *testing.T) {
	arena := NewCircularByteArena(1024)

	res1, _ := arena.TryReserve(100)
	arena.TryReserve(200)
	arena.Release(res1)

	arena.Reset()

	if arena.Claimed() != 0 || arena.Released() != 0 {
		t.Errorf("Expected zero cursors after reset, got claimed=%d released=%d",
			arena.Claimed(), arena.Released())
	}
}

func TestSimpleByteArena_BoundedAndAligned(t *testing.T) {
	arena := NewSimpleByteArena(128)

	res1, ok := arena.TryReserve(60)
	if !ok || res1.Offset != 0 {
		t.Fatalf("expected reservation at 0, got %+v ok=%v", res1, ok)
	}
	if res1.End-res1.Start != 64 {
		t.Errorf("expected 8-byte rounded range of 64, got %d", res1.End-res1.Start)
	}

	res2, ok := arena.TryReserve(64)
	if !ok || res2.Offset != 64 {
		t.Fatalf("expected reservation at 64, got %+v ok=%v", res2, ok)
	}

	// Full: must refuse rather than overwrite.
	if _, ok := arena.TryReserve(8); ok {
		t.Fatal("reservation on a full SimpleByteArena must fail")
	}

	arena.Release(res1)
	res3, ok := arena.TryReserve(32)
	if !ok || res3.Offset != 0 {
		t.Fatalf("expected wrap to offset 0 after release, got %+v ok=%v", res3, ok)
	}

	arena.Release(res2)
	arena.Release(res3)
	if arena.Released() != arena.Claimed() {
		t.Errorf("expected all space released, claimed=%d released=%d", arena.Claimed(), arena.Released())
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkCircularByteArena_ReserveRelease(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, ok := arena.TryReserve(64)
		if !ok {
			b.Fatal("reservation failed")
		}
		arena.Release(res)
	}
}

func BenchmarkCircularByteArena_ReserveWriteRelease(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024)
	data := make([]byte, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, ok := arena.TryReserve(64)
		if !ok {
			b.Fatal("reservation failed")
		}
		copy(arena.GetSlice(res.Offset, 64), data)
		arena.Release(res)
	}
}
