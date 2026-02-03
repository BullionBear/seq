package event

type Topic int

const (
	TopicEventUnknown   Topic = iota
	TopicEventUnhandled       // Known but not handled
	// Market Data
	TopicEventDepthSnapshot
	TopicEventReqDepthSnapshot
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
	TopicEventReqBalanceSnapshot
	TopicEventBalanceUpdate
	// Command
	TopicCommandOrderSubmit
	TopicCommandOrderCancel
	TopicCommandCancelAll
)
