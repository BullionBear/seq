package event

type Topic int

const (
	TopicEventUnknown   Topic = iota
	TopicEventUnhandled       // Known but not handled
	// State Topic
	TopicEventAbnormal
	TopicEventReady
	TopicEventStop
	TopicEventFinished
	// Market Data
	TopicEventDepthSnapshot
	TopicEventRespDepthSnapshot
	TopicEventDepthUpdate
	TopicEventTick
	// Execution Data
	TopicEventOrderUnknownStatus
	TopicEventOrderError
	TopicEventOrderAccepted
	TopicEventPartialFill
	TopicEventFill
	TopicEventOrderCanceled
	TopicEventOrderRejected
	// Reconciliation Data
	TopicEventRespBalanceSnapshot
	TopicEventBalanceUpdate
)
