package sma

import (
	"math"
	"testing"
)

func TestCloseRingWarmupAndOverwrite(t *testing.T) {
	r := newCloseRing(3)

	if _, ready := r.push(1); ready {
		t.Fatal("expected not ready after 1 sample")
	}
	if _, ready := r.push(2); ready {
		t.Fatal("expected not ready after 2 samples")
	}
	sma, ready := r.push(3)
	if !ready {
		t.Fatal("expected ready after 3 samples")
	}
	if math.Abs(sma-2) > 1e-12 {
		t.Fatalf("sma = %v, want 2", sma)
	}

	// Overwrite oldest (1) with 6 → window is 2,3,6
	sma, ready = r.push(6)
	if !ready {
		t.Fatal("expected ready after overwrite")
	}
	want := (2.0 + 3 + 6) / 3
	if math.Abs(sma-want) > 1e-12 {
		t.Fatalf("sma = %v, want %v", sma, want)
	}
}

func TestCloseRingPeriodOne(t *testing.T) {
	r := newCloseRing(1)
	sma, ready := r.push(42)
	if !ready {
		t.Fatal("expected ready for period 1")
	}
	if sma != 42 {
		t.Fatalf("sma = %v, want 42", sma)
	}
	sma, _ = r.push(7)
	if sma != 7 {
		t.Fatalf("sma = %v, want 7", sma)
	}
}
