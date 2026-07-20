package sma

// closeRing is a fixed-size circular buffer of close prices for SMA.
type closeRing struct {
	closes []float64
	curr   int // next write index
	count  int // filled samples, capped at len(closes)
	sum    float64
}

func newCloseRing(period int) *closeRing {
	return &closeRing{
		closes: make([]float64, period),
	}
}

// push stores close at curr, advances curr, and returns (sma, ready).
// When full, the oldest sample is overwritten.
func (r *closeRing) push(close float64) (sma float64, ready bool) {
	n := len(r.closes)
	if n == 0 {
		return 0, false
	}
	if r.count == n {
		r.sum -= r.closes[r.curr]
	}
	r.closes[r.curr] = close
	r.sum += close
	r.curr = (r.curr + 1) % n
	if r.count < n {
		r.count++
	}
	if r.count < n {
		return 0, false
	}
	return r.sum / float64(n), true
}

func (r *closeRing) filled() int { return r.count }

func (r *closeRing) period() int { return len(r.closes) }
