package macd

import (
	"math"
	"testing"
)

func TestEMASeedsWithSMA(t *testing.T) {
	e := newEMA(3)
	if _, ready := e.Update(1); ready {
		t.Fatal("expected not ready after 1 sample")
	}
	if _, ready := e.Update(2); ready {
		t.Fatal("expected not ready after 2 samples")
	}
	v, ready := e.Update(3)
	if !ready {
		t.Fatal("expected ready after 3 samples")
	}
	if math.Abs(v-2.0) > 1e-12 {
		t.Fatalf("SMA seed = %v, want 2", v)
	}
}

func TestMACDCrossover(t *testing.T) {
	// Short periods for a deterministic synthetic series.
	st := newMACDState(3, 6, 3)

	// Downtrend first so hist is negative, then sharp uptrend (bullish),
	// then downtrend again (bearish).
	closes := make([]float64, 0, 40)
	for i := 0; i < 12; i++ {
		closes = append(closes, 100-float64(i)) // decline
	}
	for i := 0; i < 12; i++ {
		closes = append(closes, 88+float64(i)*3) // sharp rise
	}
	for i := 0; i < 12; i++ {
		closes = append(closes, 124-float64(i)*3) // sharp fall
	}

	var sawBullish, sawBearish bool
	for _, c := range closes {
		_, _, _, ready, xover := st.Update(c)
		if !ready {
			continue
		}
		switch xover {
		case crossoverBullish:
			sawBullish = true
		case crossoverBearish:
			if sawBullish {
				sawBearish = true
			}
		}
	}
	if !sawBullish {
		t.Fatal("expected bullish crossover after uptrend")
	}
	if !sawBearish {
		t.Fatal("expected bearish crossover after downtrend")
	}
}

func TestMACDNotReadyUntilSlowAndSignal(t *testing.T) {
	st := newMACDState(2, 4, 3)
	readyCount := 0
	for i := 0; i < 20; i++ {
		_, _, _, ready, _ := st.Update(float64(100 + i))
		if ready {
			readyCount++
		}
	}
	if readyCount == 0 {
		t.Fatal("expected indicator to become ready")
	}
	// First ready bar requires slow SMA (4) + signal SMA (3) after MACD exists.
	st2 := newMACDState(2, 4, 3)
	firstReady := -1
	for i := 0; i < 20; i++ {
		_, _, _, ready, _ := st2.Update(100)
		if ready {
			firstReady = i + 1
			break
		}
	}
	if firstReady != 6 { // slow=4 then signal needs 3 MACD samples → bar 6
		t.Fatalf("first ready at bar %d, want 6", firstReady)
	}
}
