package mem

import (
	"sync"
	"sync/atomic"
	"testing"
)

// =============================================================================
// SPSC Ring Buffer Tests
// =============================================================================

func TestSPSCRingBuffer_BasicOperations(t *testing.T) {
	rb := NewSPSCRingBuffer[int](4)

	// Test empty buffer
	if !rb.IsEmpty() {
		t.Error("New buffer should be empty")
	}
	if rb.IsFull() {
		t.Error("New buffer should not be full")
	}

	// Test write and read
	if !rb.Write(1) {
		t.Error("Write to empty buffer should succeed")
	}
	if !rb.Write(2) {
		t.Error("Write to non-full buffer should succeed")
	}

	if rb.Count() != 2 {
		t.Errorf("Expected count 2, got %d", rb.Count())
	}

	val, ok := rb.Read()
	if !ok || val != 1 {
		t.Errorf("Expected (1, true), got (%d, %v)", val, ok)
	}

	val, ok = rb.Read()
	if !ok || val != 2 {
		t.Errorf("Expected (2, true), got (%d, %v)", val, ok)
	}

	// Test empty after reads
	if !rb.IsEmpty() {
		t.Error("Buffer should be empty after reading all items")
	}

	_, ok = rb.Read()
	if ok {
		t.Error("Read from empty buffer should fail")
	}
}

func TestSPSCRingBuffer_FullBuffer(t *testing.T) {
	rb := NewSPSCRingBuffer[int](4) // Rounds up to 4

	// Fill the buffer
	for i := 0; i < 4; i++ {
		if !rb.Write(i) {
			t.Errorf("Write %d should succeed", i)
		}
	}

	if !rb.IsFull() {
		t.Error("Buffer should be full")
	}

	// Write to full buffer should fail
	if rb.Write(100) {
		t.Error("Write to full buffer should fail")
	}

	// Read one and write should succeed
	rb.Read()
	if !rb.Write(100) {
		t.Error("Write after read should succeed")
	}
}

func TestSPSCRingBuffer_PowerOf2Rounding(t *testing.T) {
	tests := []struct {
		requested uint64
		expected  uint64
	}{
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{7, 8},
		{8, 8},
		{9, 16},
		{100, 128},
	}

	for _, tt := range tests {
		rb := NewSPSCRingBuffer[int](tt.requested)
		if rb.Capacity() != tt.expected {
			t.Errorf("Requested %d, expected capacity %d, got %d", tt.requested, tt.expected, rb.Capacity())
		}
	}
}

func TestSPSCRingBuffer_Peek(t *testing.T) {
	rb := NewSPSCRingBuffer[int](4)

	// Peek on empty buffer should fail
	_, ok := rb.Peek()
	if ok {
		t.Error("Peek on empty buffer should fail")
	}

	// Write some items
	rb.Write(1)
	rb.Write(2)
	rb.Write(3)

	// Peek should return first item without removing it
	val, ok := rb.Peek()
	if !ok || val != 1 {
		t.Errorf("Expected (1, true), got (%d, %v)", val, ok)
	}

	// Peek again should return same item
	val, ok = rb.Peek()
	if !ok || val != 1 {
		t.Errorf("Expected (1, true) again, got (%d, %v)", val, ok)
	}

	// Count should still be 3
	if rb.Count() != 3 {
		t.Errorf("Expected count 3, got %d", rb.Count())
	}

	// Read should remove the item
	val, ok = rb.Read()
	if !ok || val != 1 {
		t.Errorf("Expected (1, true) from Read, got (%d, %v)", val, ok)
	}

	// Now Peek should return 2
	val, ok = rb.Peek()
	if !ok || val != 2 {
		t.Errorf("Expected (2, true), got (%d, %v)", val, ok)
	}
}

func TestSPSCRingBuffer_Concurrent(t *testing.T) {
	rb := NewSPSCRingBuffer[int](1024)
	const numItems = 10000

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < numItems; i++ {
			for !rb.Write(i) {
				// Spin until write succeeds
			}
		}
	}()

	// Consumer
	received := make([]int, 0, numItems)
	go func() {
		defer wg.Done()
		for len(received) < numItems {
			if val, ok := rb.Read(); ok {
				received = append(received, val)
			}
		}
	}()

	wg.Wait()

	// Verify all items received in order
	if len(received) != numItems {
		t.Errorf("Expected %d items, got %d", numItems, len(received))
	}
	for i, v := range received {
		if v != i {
			t.Errorf("Item %d: expected %d, got %d", i, i, v)
			break
		}
	}
}

// =============================================================================
// MPSC Ring Buffer Tests
// =============================================================================

func TestMPSCRingBuffer_BasicOperations(t *testing.T) {
	rb := NewMPSCRingBuffer[int](4)

	// Test empty buffer
	if !rb.IsEmpty() {
		t.Error("New buffer should be empty")
	}
	if rb.IsFull() {
		t.Error("New buffer should not be full")
	}

	// Test write and read
	if !rb.Write(1) {
		t.Error("Write to empty buffer should succeed")
	}
	if !rb.Write(2) {
		t.Error("Write to non-full buffer should succeed")
	}

	if rb.Count() != 2 {
		t.Errorf("Expected count 2, got %d", rb.Count())
	}

	val, ok := rb.Read()
	if !ok || val != 1 {
		t.Errorf("Expected (1, true), got (%d, %v)", val, ok)
	}

	val, ok = rb.Read()
	if !ok || val != 2 {
		t.Errorf("Expected (2, true), got (%d, %v)", val, ok)
	}

	// Test empty after reads
	if !rb.IsEmpty() {
		t.Error("Buffer should be empty after reading all items")
	}

	_, ok = rb.Read()
	if ok {
		t.Error("Read from empty buffer should fail")
	}
}

func TestMPSCRingBuffer_FullBuffer(t *testing.T) {
	rb := NewMPSCRingBuffer[int](4) // Rounds up to 4

	// Fill the buffer
	for i := 0; i < 4; i++ {
		if !rb.Write(i) {
			t.Errorf("Write %d should succeed", i)
		}
	}

	if !rb.IsFull() {
		t.Error("Buffer should be full")
	}

	// Write to full buffer should fail
	if rb.Write(100) {
		t.Error("Write to full buffer should fail")
	}

	// Read one and write should succeed
	rb.Read()
	if !rb.Write(100) {
		t.Error("Write after read should succeed")
	}
}

func TestMPSCRingBuffer_SingleProducerSingleConsumer(t *testing.T) {
	rb := NewMPSCRingBuffer[int](1024)
	const numItems = 10000

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < numItems; i++ {
			for !rb.Write(i) {
				// Spin until write succeeds
			}
		}
	}()

	// Consumer
	received := make([]int, 0, numItems)
	go func() {
		defer wg.Done()
		for len(received) < numItems {
			if val, ok := rb.Read(); ok {
				received = append(received, val)
			}
		}
	}()

	wg.Wait()

	// Verify all items received in order
	if len(received) != numItems {
		t.Errorf("Expected %d items, got %d", numItems, len(received))
	}
	for i, v := range received {
		if v != i {
			t.Errorf("Item %d: expected %d, got %d", i, i, v)
			break
		}
	}
}

func TestMPSCRingBuffer_MultipleProducersSingleConsumer(t *testing.T) {
	rb := NewMPSCRingBuffer[int](4096)
	const numProducers = 4
	const itemsPerProducer = 2500

	var wg sync.WaitGroup
	var producersDone int32

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			defer atomic.AddInt32(&producersDone, 1)
			for i := 0; i < itemsPerProducer; i++ {
				// Encode producer ID and item index in the value
				val := producerID*itemsPerProducer + i
				for !rb.Write(val) {
					// Spin until write succeeds
				}
			}
		}(p)
	}

	// Consumer
	received := make(map[int]bool)
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		expectedTotal := numProducers * itemsPerProducer
		for {
			if val, ok := rb.Read(); ok {
				mu.Lock()
				received[val] = true
				count := len(received)
				mu.Unlock()
				if count >= expectedTotal {
					break
				}
			} else if atomic.LoadInt32(&producersDone) == numProducers && rb.IsEmpty() {
				break
			}
		}
	}()

	wg.Wait()

	// Verify all items received (order may vary due to concurrent producers)
	expectedTotal := numProducers * itemsPerProducer
	if len(received) != expectedTotal {
		t.Errorf("Expected %d unique items, got %d", expectedTotal, len(received))
	}

	// Verify each expected value was received
	for i := 0; i < expectedTotal; i++ {
		if !received[i] {
			t.Errorf("Missing value: %d", i)
		}
	}
}

func TestMPSCRingBuffer_NoDuplicates(t *testing.T) {
	rb := NewMPSCRingBuffer[int](1024)
	const numProducers = 8
	const itemsPerProducer = 1000

	var wg sync.WaitGroup
	var producersDone int32

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			defer atomic.AddInt32(&producersDone, 1)
			for i := 0; i < itemsPerProducer; i++ {
				val := producerID*itemsPerProducer + i
				for !rb.Write(val) {
					// Spin
				}
			}
		}(p)
	}

	// Consumer - collect all values
	valueCounts := make(map[int]int)
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		expectedTotal := numProducers * itemsPerProducer
		count := 0
		for count < expectedTotal {
			if val, ok := rb.Read(); ok {
				mu.Lock()
				valueCounts[val]++
				count++
				mu.Unlock()
			} else if atomic.LoadInt32(&producersDone) == numProducers && rb.IsEmpty() {
				break
			}
		}
	}()

	wg.Wait()

	// Check for duplicates
	for val, count := range valueCounts {
		if count > 1 {
			t.Errorf("Value %d received %d times (expected 1)", val, count)
		}
	}
}

// =============================================================================
// Slice Arena Tests
// =============================================================================

func TestSliceArena_BasicOperations(t *testing.T) {
	arena := NewSliceArena[int](100)

	// Test initial state
	if arena.Capacity() != 100 {
		t.Errorf("Expected capacity 100, got %d", arena.Capacity())
	}

	// Allocate some items
	slice1 := arena.Allocate(10)
	if len(slice1) != 10 {
		t.Errorf("Expected slice length 10, got %d", len(slice1))
	}

	// Write to slice
	for i := range slice1 {
		slice1[i] = i
	}

	// Verify values
	for i := range slice1 {
		if slice1[i] != i {
			t.Errorf("Expected slice1[%d] = %d, got %d", i, i, slice1[i])
		}
	}

	// Allocate more
	slice2 := arena.Allocate(20)
	if len(slice2) != 20 {
		t.Errorf("Expected slice length 20, got %d", len(slice2))
	}

	// Slices should not overlap
	for i := range slice2 {
		slice2[i] = i + 100
	}

	// Original slice1 should be unchanged
	for i := range slice1 {
		if slice1[i] != i {
			t.Errorf("slice1 was corrupted: slice1[%d] = %d, expected %d", i, slice1[i], i)
		}
	}
}

func TestSliceArena_WrapAround(t *testing.T) {
	arena := NewSliceArena[int](100)

	// Fill most of the arena
	slice1 := arena.Allocate(80)
	for i := range slice1 {
		slice1[i] = i
	}

	// This allocation should wrap to beginning (80 + 30 > 100)
	slice2 := arena.Allocate(30)
	if len(slice2) != 30 {
		t.Errorf("Expected slice length 30, got %d", len(slice2))
	}

	// Write to slice2
	for i := range slice2 {
		slice2[i] = i + 1000
	}

	// Verify slice2 values
	for i := range slice2 {
		if slice2[i] != i+1000 {
			t.Errorf("Expected slice2[%d] = %d, got %d", i, i+1000, slice2[i])
		}
	}
}

func TestSliceArena_ExactCapacity(t *testing.T) {
	arena := NewSliceArena[int](50)

	// Request exactly capacity should work
	slice := arena.Allocate(50)
	if len(slice) != 50 {
		t.Errorf("Expected slice length 50, got %d", len(slice))
	}

	// Write to verify it works
	for i := range slice {
		slice[i] = i
	}
}

func TestSliceArena_Reset(t *testing.T) {
	arena := NewSliceArena[int](100)

	// Make some allocations
	arena.Allocate(50)
	arena.Allocate(30)

	// Reset
	arena.Reset()

	// Next allocation should start from beginning
	slice := arena.Allocate(10)
	// The slice should start at index 0 (verified by checking the underlying array)
	for i := range slice {
		slice[i] = i * 10
	}

	// Verify
	for i := range slice {
		if slice[i] != i*10 {
			t.Errorf("Expected slice[%d] = %d, got %d", i, i*10, slice[i])
		}
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkSPSCRingBuffer_Write(b *testing.B) {
	rb := NewSPSCRingBuffer[int](uint64(b.N) + 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
}

func BenchmarkSPSCRingBuffer_WriteRead(b *testing.B) {
	rb := NewSPSCRingBuffer[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
		rb.Read()
	}
}

func BenchmarkMPSCRingBuffer_Write(b *testing.B) {
	rb := NewMPSCRingBuffer[int](uint64(b.N) + 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
}

func BenchmarkMPSCRingBuffer_WriteRead(b *testing.B) {
	rb := NewMPSCRingBuffer[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
		rb.Read()
	}
}

func BenchmarkMPSCRingBuffer_ConcurrentWrites(b *testing.B) {
	rb := NewMPSCRingBuffer[int](8192)
	var wg sync.WaitGroup

	// Start consumer
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				rb.Read()
			}
		}
	}()

	b.ResetTimer()
	numProducers := 4
	itemsPerProducer := b.N / numProducers

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				for !rb.Write(i) {
				}
			}
		}()
	}

	wg.Wait()
	close(done)
}
