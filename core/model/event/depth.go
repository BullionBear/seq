package event

import "github.com/BullionBear/seq/core/model/common"

type DepthUpdate struct {
	SymbolID        int
	PreviousDepthID int
	DepthID         int
	CurrentDepthID  int
	NextDepthID     int
	Timestamp       uint64
	Asks            []common.PriceLevel
	Bids            []common.PriceLevel
}

func (d DepthUpdate) Topic() Topic {
	return TopicEventDepthUpdate
}

type DepthSnapshot struct {
	SymbolID  int
	DepthID   int
	Timestamp uint64
	Asks      []common.PriceLevel
	Bids      []common.PriceLevel
}

func (d DepthSnapshot) Topic() Topic {
	return TopicEventDepthSnapshot
}

type RespDepthSnapshot struct {
	SymbolID  int
	DepthID   int
	Timestamp uint64
	AskLength int                 // number of ask price levels
	BidLength int                 // number of bid price levels
	Asks      []common.PriceLevel // points into arena buffer
	Bids      []common.PriceLevel // points into arena buffer
}

func (r RespDepthSnapshot) Topic() Topic {
	return TopicEventRespDepthSnapshot
}
