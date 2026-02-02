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
	TopicEventBalanceSnapshot
	TopicEventBalanceUpdate
	// Command
	TopicCommandOrderSubmit
	TopicCommandOrderCancel
	TopicCommandCancelAll
)
