package event

import "fmt"

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
	TopicEventOrderRiskInvalid
	TopicEventOrderNew
	TopicEventOrderAccepted
	TopicEventPartialFill
	TopicEventFill
	TopicEventOrderCanceled
	TopicEventOrderRejected
	// Reconciliation Data
	TopicEventRespBalanceSnapshot
	TopicEventBalanceUpdate
)

func (t Topic) String() string {
	switch t {
	case TopicEventUnknown:
		return "Unknown"
	case TopicEventUnhandled:
		return "Unhandled"
	case TopicEventAbnormal:
		return "Abnormal"
	case TopicEventReady:
		return "Ready"
	case TopicEventStop:
		return "Stop"
	case TopicEventFinished:
		return "Finished"
	case TopicEventDepthSnapshot:
		return "DepthSnapshot"
	case TopicEventRespDepthSnapshot:
		return "RespDepthSnapshot"
	case TopicEventDepthUpdate:
		return "DepthUpdate"
	case TopicEventTick:
		return "Tick"
	case TopicEventOrderRiskInvalid:
		return "OrderRiskInvalid"
	case TopicEventOrderUnknownStatus:
		return "OrderUnknownStatus"
	case TopicEventOrderError:
		return "OrderError"
	case TopicEventOrderNew:
		return "OrderNew"
	case TopicEventOrderAccepted:
		return "OrderAccepted"
	case TopicEventPartialFill:
		return "PartialFill"
	case TopicEventFill:
		return "Fill"
	case TopicEventOrderCanceled:
		return "OrderCanceled"
	case TopicEventOrderRejected:
		return "OrderRejected"
	case TopicEventRespBalanceSnapshot:
		return "RespBalanceSnapshot"
	case TopicEventBalanceUpdate:
		return "BalanceUpdate"
	default:
		return fmt.Sprintf("Undefined(%d)", int(t))
	}
}
