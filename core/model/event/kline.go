package event

import "github.com/BullionBear/seq/core/model/common"

// Kline is a candlestick / OHLCV update from a venue kline stream.
type Kline struct {
	SymbolID    int
	Interval    common.Interval
	StartTime   uint64 // candle open time, nanoseconds
	EndTime     uint64 // candle close time, nanoseconds
	Timestamp   uint64 // venue event / last-match time, nanoseconds
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64 // base asset volume
	QuoteVolume float64 // quote asset volume / turnover
	TradeCount  int
	Closed      bool // true when the candle is finalized
}

func (k Kline) Topic() Topic {
	return TopicEventKline
}

// KlineBar is one OHLCV bar inside RespHistoricalKline (no symbol/interval;
// those live on the response header). Times are nanoseconds.
type KlineBar struct {
	StartTime   uint64
	EndTime     uint64
	Timestamp   uint64
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	TradeCount  int
	Closed      bool
}

// RespHistoricalKline is the reply to CommandTypeReqHistoricalKline.
// Bars are ordered oldest → newest.
type RespHistoricalKline struct {
	SymbolID int
	Interval common.Interval
	Bars     []KlineBar // may alias the arena buffer until the handler returns
}

func (r RespHistoricalKline) Topic() Topic {
	return TopicEventRespHistoricalKline
}
