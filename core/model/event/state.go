package event

import "github.com/BullionBear/seq/core/model/common"

type StopEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

func (s StopEvent) Topic() Topic {
	return TopicEventStop
}

type ReadyEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

func (r ReadyEvent) Topic() Topic {
	return TopicEventReady
}

type FinishedEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

func (f FinishedEvent) Topic() Topic {
	return TopicEventFinished
}

type AbnormalEvent struct {
	Source    common.EngineType
	ErrorCode int
	Timestamp uint64
}

func (a AbnormalEvent) Topic() Topic {
	return TopicEventAbnormal
}
