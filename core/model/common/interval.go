package common

import "fmt"

// Interval is a normalized candlestick interval shared across venues.
type Interval int

const (
	IntervalUnknown Interval = iota
	Interval1s
	Interval1m
	Interval3m
	Interval5m
	Interval15m
	Interval30m
	Interval1h
	Interval2h
	Interval4h
	Interval6h
	Interval8h
	Interval12h
	Interval1d
	Interval3d
	Interval1w
	Interval1M
)

// ParseInterval converts a config / venue interval string to Interval.
// Accepts Binance forms (1m, 1h, 1d) and Bybit forms (1, 60, D, W, M).
func ParseInterval(s string) (Interval, error) {
	switch s {
	case "1s":
		return Interval1s, nil
	case "1m", "1":
		return Interval1m, nil
	case "3m", "3":
		return Interval3m, nil
	case "5m", "5":
		return Interval5m, nil
	case "15m", "15":
		return Interval15m, nil
	case "30m", "30":
		return Interval30m, nil
	case "1h", "60":
		return Interval1h, nil
	case "2h", "120":
		return Interval2h, nil
	case "4h", "240":
		return Interval4h, nil
	case "6h", "360":
		return Interval6h, nil
	case "8h":
		return Interval8h, nil
	case "12h", "720":
		return Interval12h, nil
	case "1d", "D", "d":
		return Interval1d, nil
	case "3d":
		return Interval3d, nil
	case "1w", "W", "w":
		return Interval1w, nil
	case "1M", "M":
		return Interval1M, nil
	default:
		return IntervalUnknown, fmt.Errorf("unknown interval: %q", s)
	}
}

func (i Interval) String() string {
	switch i {
	case Interval1s:
		return "1s"
	case Interval1m:
		return "1m"
	case Interval3m:
		return "3m"
	case Interval5m:
		return "5m"
	case Interval15m:
		return "15m"
	case Interval30m:
		return "30m"
	case Interval1h:
		return "1h"
	case Interval2h:
		return "2h"
	case Interval4h:
		return "4h"
	case Interval6h:
		return "6h"
	case Interval8h:
		return "8h"
	case Interval12h:
		return "12h"
	case Interval1d:
		return "1d"
	case Interval3d:
		return "3d"
	case Interval1w:
		return "1w"
	case Interval1M:
		return "1M"
	default:
		return "Unknown"
	}
}

// BinanceStream returns the Binance kline stream interval suffix (e.g. "1m").
func (i Interval) BinanceStream() string {
	return i.String()
}

// BybitTopic returns the Bybit kline topic interval token (e.g. "1", "60", "D").
func (i Interval) BybitTopic() (string, bool) {
	switch i {
	case Interval1m:
		return "1", true
	case Interval3m:
		return "3", true
	case Interval5m:
		return "5", true
	case Interval15m:
		return "15", true
	case Interval30m:
		return "30", true
	case Interval1h:
		return "60", true
	case Interval2h:
		return "120", true
	case Interval4h:
		return "240", true
	case Interval6h:
		return "360", true
	case Interval12h:
		return "720", true
	case Interval1d:
		return "D", true
	case Interval1w:
		return "W", true
	case Interval1M:
		return "M", true
	default:
		return "", false
	}
}
