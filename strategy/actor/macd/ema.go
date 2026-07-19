package macd

// ema is an exponential moving average seeded with an SMA of the first period samples.
type ema struct {
	period int
	mult   float64
	value  float64
	sum    float64
	count  int
	ready  bool
}

func newEMA(period int) *ema {
	if period < 1 {
		period = 1
	}
	return &ema{
		period: period,
		mult:   2.0 / float64(period+1),
	}
}

// Update feeds one sample. ready becomes true once the SMA seed is complete.
func (e *ema) Update(x float64) (value float64, ready bool) {
	if e.ready {
		e.value = x*e.mult + e.value*(1-e.mult)
		return e.value, true
	}
	e.sum += x
	e.count++
	if e.count < e.period {
		return 0, false
	}
	e.value = e.sum / float64(e.period)
	e.ready = true
	return e.value, true
}

// macdState tracks MACD line and signal line from close prices.
type macdState struct {
	fast   *ema
	slow   *ema
	signal *ema

	prevDiff float64
	haveDiff bool
}

func newMACDState(fast, slow, signal int) *macdState {
	return &macdState{
		fast:   newEMA(fast),
		slow:   newEMA(slow),
		signal: newEMA(signal),
	}
}

type crossover int

const (
	crossoverNone crossover = iota
	crossoverBullish
	crossoverBearish
)

// Update consumes a closed-bar close price and returns MACD components plus any crossover.
func (m *macdState) Update(close float64) (macdLine, signalLine, hist float64, ready bool, xover crossover) {
	fastVal, fastReady := m.fast.Update(close)
	slowVal, slowReady := m.slow.Update(close)
	if !fastReady || !slowReady {
		return 0, 0, 0, false, crossoverNone
	}

	macdLine = fastVal - slowVal
	signalLine, signalReady := m.signal.Update(macdLine)
	if !signalReady {
		return macdLine, 0, 0, false, crossoverNone
	}

	hist = macdLine - signalLine
	xover = crossoverNone
	if m.haveDiff {
		if m.prevDiff <= 0 && hist > 0 {
			xover = crossoverBullish
		} else if m.prevDiff >= 0 && hist < 0 {
			xover = crossoverBearish
		}
	}
	m.prevDiff = hist
	m.haveDiff = true
	return macdLine, signalLine, hist, true, xover
}
