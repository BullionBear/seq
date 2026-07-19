package command

import "github.com/BullionBear/seq/core/model/common"

// ReqHistoricalKline requests historical candlesticks via the data engine.
// StartTime/EndTime are nanoseconds; 0 means omit that bound.
// Limit is the max number of bars (venue-capped); 0 uses the venue default.
type ReqHistoricalKline struct {
	SymbolID  int
	Interval  common.Interval
	StartTime uint64
	EndTime   uint64
	Limit     int
}

func (r ReqHistoricalKline) CommandType() CommandType {
	return CommandTypeReqHistoricalKline
}
