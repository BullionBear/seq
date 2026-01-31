package mem

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestCircularByteArena_BasicOperations(t *testing.T) {
	arena := NewCircularByteArena(1024)

	// Test initial state
	if arena.WriteOff() != 0 {
		t.Errorf("Expected writeOff 0, got %d", arena.WriteOff())
	}
	if arena.ReadOff() != 0 {
		t.Errorf("Expected readOff 0, got %d", arena.ReadOff())
	}
	if arena.Capacity() != 1024 {
		t.Errorf("Expected capacity 1024, got %d", arena.Capacity())
	}

	// Test Reserve
	offset := arena.Reserve(100)
	if offset != 0 {
		t.Errorf("Expected offset 0, got %d", offset)
	}
	if arena.WriteOff() != 100 {
		t.Errorf("Expected writeOff 100, got %d", arena.WriteOff())
	}

	// Test GetSlice and WriteAt
	slice := arena.GetSlice(offset, 100)
	if len(slice) != 100 {
		t.Errorf("Expected slice length 100, got %d", len(slice))
	}

	// Write some data
	testData := []byte("hello world")
	arena.WriteAt(offset, testData)

	// Test ReadSlice
	readSlice := arena.ReadSlice(offset, uint64(len(testData)))
	if string(readSlice) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(readSlice))
	}
}

func TestCircularByteArena_WrapAround(t *testing.T) {
	arena := NewCircularByteArena(100)

	// Fill most of the buffer
	offset1 := arena.Reserve(80)
	if offset1 != 0 {
		t.Errorf("Expected offset 0, got %d", offset1)
	}

	// This allocation should wrap to beginning
	offset2 := arena.Reserve(30)
	if offset2 != 0 {
		t.Errorf("Expected wrap to offset 0, got %d", offset2)
	}
	if arena.WriteOff() != 30 {
		t.Errorf("Expected writeOff 30, got %d", arena.WriteOff())
	}
}

func TestCircularByteArena_MultipleReserves(t *testing.T) {
	arena := NewCircularByteArena(1024)

	offsets := make([]uint64, 10)
	expectedOffset := uint64(0)

	for i := 0; i < 10; i++ {
		offsets[i] = arena.Reserve(100)
		if offsets[i] != expectedOffset {
			t.Errorf("Reserve %d: expected offset %d, got %d", i, expectedOffset, offsets[i])
		}
		expectedOffset += 100
	}

	if arena.WriteOff() != 1000 {
		t.Errorf("Expected writeOff 1000, got %d", arena.WriteOff())
	}
}

func TestCircularByteArena_ConcurrentReserve(t *testing.T) {
	arena := NewCircularByteArena(1024 * 1024) // 1MB
	const numGoroutines = 8
	const reservesPerGoroutine = 1000
	const reserveSize = 64

	var wg sync.WaitGroup
	offsets := make([][]uint64, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		offsets[g] = make([]uint64, reservesPerGoroutine)
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < reservesPerGoroutine; i++ {
				offsets[goroutineID][i] = arena.Reserve(reserveSize)
			}
		}(g)
	}

	wg.Wait()

	// Verify no overlapping allocations
	// Collect all [offset, offset+size) ranges
	type allocation struct {
		start uint64
		end   uint64
	}
	allocs := make([]allocation, 0, numGoroutines*reservesPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		for i := 0; i < reservesPerGoroutine; i++ {
			allocs = append(allocs, allocation{
				start: offsets[g][i],
				end:   offsets[g][i] + reserveSize,
			})
		}
	}

	// Check for overlaps (simple O(n²) check for correctness)
	for i := 0; i < len(allocs); i++ {
		for j := i + 1; j < len(allocs); j++ {
			a, b := allocs[i], allocs[j]
			// Two ranges [a.start, a.end) and [b.start, b.end) overlap if
			// a.start < b.end && b.start < a.end
			if a.start < b.end && b.start < a.end {
				t.Errorf("Overlapping allocations: [%d,%d) and [%d,%d)",
					a.start, a.end, b.start, b.end)
			}
		}
	}
}

func TestCircularByteArena_ConcurrentWriteRead(t *testing.T) {
	arena := NewCircularByteArena(1024 * 1024) // 1MB
	const numProducers = 4
	const itemsPerProducer = 1000

	type item struct {
		offset uint64
		size   uint64
		data   []byte
	}

	var wg sync.WaitGroup
	items := make(chan item, numProducers*itemsPerProducer)

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				// Create unique data for this producer/item
				data := []byte{byte(producerID), byte(i >> 8), byte(i & 0xFF)}
				size := uint64(len(data))

				offset := arena.Reserve(size)
				slice := arena.GetSlice(offset, size)
				copy(slice, data)

				items <- item{offset: offset, size: size, data: data}
			}
		}(p)
	}

	// Wait for producers and close channel
	go func() {
		wg.Wait()
		close(items)
	}()

	// Consumer: verify all items
	count := 0
	for it := range items {
		readData := arena.ReadSlice(it.offset, it.size)
		if len(readData) != len(it.data) {
			t.Errorf("Size mismatch: expected %d, got %d", len(it.data), len(readData))
		}
		for j := range it.data {
			if readData[j] != it.data[j] {
				t.Errorf("Data mismatch at item %d, byte %d: expected %d, got %d",
					count, j, it.data[j], readData[j])
			}
		}
		count++
	}

	expectedCount := numProducers * itemsPerProducer
	if count != expectedCount {
		t.Errorf("Expected %d items, got %d", expectedCount, count)
	}
}

func TestCircularByteArena_AdvanceRead(t *testing.T) {
	arena := NewCircularByteArena(1024)

	// Reserve some space
	arena.Reserve(100)
	arena.Reserve(100)

	// Advance read
	arena.AdvanceRead(100)
	if arena.ReadOff() != 100 {
		t.Errorf("Expected readOff 100, got %d", arena.ReadOff())
	}

	// Advance read further
	arena.AdvanceRead(200)
	if arena.ReadOff() != 200 {
		t.Errorf("Expected readOff 200, got %d", arena.ReadOff())
	}

	// Advancing backwards should be ignored
	arena.AdvanceRead(100)
	if arena.ReadOff() != 200 {
		t.Errorf("Expected readOff to stay at 200, got %d", arena.ReadOff())
	}
}

func TestCircularByteArena_Reset(t *testing.T) {
	arena := NewCircularByteArena(1024)

	// Make some allocations
	arena.Reserve(100)
	arena.Reserve(200)
	arena.AdvanceRead(100)

	// Reset
	arena.Reset()

	if arena.WriteOff() != 0 {
		t.Errorf("Expected writeOff 0 after reset, got %d", arena.WriteOff())
	}
	if arena.ReadOff() != 0 {
		t.Errorf("Expected readOff 0 after reset, got %d", arena.ReadOff())
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkCircularByteArena_Reserve(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024 * 10) // 10MB
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		arena.Reserve(64)
	}
}

func BenchmarkCircularByteArena_ReserveAndWrite(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024 * 10) // 10MB
	data := make([]byte, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := arena.Reserve(64)
		slice := arena.GetSlice(offset, 64)
		copy(slice, data)
	}
}

func BenchmarkCircularByteArena_ConcurrentReserve(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024 * 100) // 100MB
	var wg sync.WaitGroup
	numProducers := 4
	itemsPerProducer := b.N / numProducers

	b.ResetTimer()
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				arena.Reserve(64)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkCircularByteArena_ConcurrentReserveAndWrite(b *testing.B) {
	arena := NewCircularByteArena(1024 * 1024 * 100) // 100MB
	var wg sync.WaitGroup
	numProducers := 4
	itemsPerProducer := b.N / numProducers

	b.ResetTimer()
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := make([]byte, 64)
			for i := 0; i < itemsPerProducer; i++ {
				offset := arena.Reserve(64)
				slice := arena.GetSlice(offset, 64)
				copy(slice, data)
			}
		}()
	}
	wg.Wait()
}

func TestCircularByteArena_NoAllocationRace(t *testing.T) {
	// This test verifies that concurrent Reserve calls never return overlapping ranges
	// by checking that writing to reserved ranges doesn't cause data corruption
	arena := NewCircularByteArena(1024 * 1024) // 1MB
	const numProducers = 8
	const itemsPerProducer = 5000
	const itemSize = 32

	var wg sync.WaitGroup
	var errors int64

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				offset := arena.Reserve(itemSize)
				slice := arena.GetSlice(offset, itemSize)

				// Write producer ID and sequence to the slice
				for j := 0; j < itemSize; j++ {
					slice[j] = byte(producerID)
				}

				// Verify the data wasn't corrupted by another producer
				for j := 0; j < itemSize; j++ {
					if slice[j] != byte(producerID) {
						atomic.AddInt64(&errors, 1)
						break
					}
				}
			}
		}(p)
	}

	wg.Wait()

	if errors > 0 {
		t.Errorf("Detected %d data corruption events - allocations overlap!", errors)
	}
}
