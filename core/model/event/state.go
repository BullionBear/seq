package event

import "github.com/BullionBear/seq/core/model/common"

type StopEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

type ReadyEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

type FinishedEvent struct {
	Source    common.EngineType
	Timestamp uint64
}

type AbnormalEvent struct {
	Source    common.EngineType
	ErrorCode int
	Timestamp uint64
}
