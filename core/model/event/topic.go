package event

type Topic int

const (
	// Market Data
	TopicEventDepthSnapshot Topic = iota
	TopicEventReqDepthSnapshot
	TopicEventDepthUpdate
	TopicEventTick
	// Execution Data
	TopicEventOrderUpdate
	TopicEventFill
	// Reconciliation Data
	TopicEventReqBalanceSnapshot
	TopicEventBalanceUpdate
	// Command
	TopicCommandOrderSubmit
	TopicCommandOrderCancel
	TopicCommandCancelAll
)
