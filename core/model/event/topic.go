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
	TopicEventOrderPartialFill
	TopicEventOrderFilled
	TopicEventExecution
	TopicEventOrderCanceled
	TopicEventOrderRejected
	// Reconciliation Data
	TopicEventRespBalanceSnapshot
	TopicEventBalanceUpdate
	// Timer
	TopicEventTimer
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
	case TopicEventOrderPartialFill:
		return "OrderPartialFill"
	case TopicEventOrderFilled:
		return "OrderFilled"
	case TopicEventExecution:
		return "Execution"
	case TopicEventOrderCanceled:
		return "OrderCanceled"
	case TopicEventOrderRejected:
		return "OrderRejected"
	case TopicEventRespBalanceSnapshot:
		return "RespBalanceSnapshot"
	case TopicEventBalanceUpdate:
		return "BalanceUpdate"
	case TopicEventTimer:
		return "Timer"
	default:
		return fmt.Sprintf("Undefined(%d)", int(t))
	}
}

// topicByName maps human-readable names (used in config) to Topic constants.
var topicByName = map[string]Topic{
	"Unknown":             TopicEventUnknown,
	"Unhandled":           TopicEventUnhandled,
	"Abnormal":            TopicEventAbnormal,
	"Ready":               TopicEventReady,
	"Stop":                TopicEventStop,
	"Finished":            TopicEventFinished,
	"DepthSnapshot":       TopicEventDepthSnapshot,
	"RespDepthSnapshot":   TopicEventRespDepthSnapshot,
	"DepthUpdate":         TopicEventDepthUpdate,
	"Tick":                TopicEventTick,
	"OrderUnknownStatus":  TopicEventOrderUnknownStatus,
	"OrderError":          TopicEventOrderError,
	"OrderRiskInvalid":    TopicEventOrderRiskInvalid,
	"OrderNew":            TopicEventOrderNew,
	"OrderAccepted":       TopicEventOrderAccepted,
	"OrderPartialFill":    TopicEventOrderPartialFill,
	"OrderFilled":         TopicEventOrderFilled,
	"Execution":           TopicEventExecution,
	"OrderCanceled":       TopicEventOrderCanceled,
	"OrderRejected":       TopicEventOrderRejected,
	"RespBalanceSnapshot": TopicEventRespBalanceSnapshot,
	"BalanceUpdate":       TopicEventBalanceUpdate,
	"Timer":               TopicEventTimer,
}

// ParseTopic converts a config string (e.g. "OrderNew") to its Topic constant.
func ParseTopic(name string) (Topic, error) {
	if t, ok := topicByName[name]; ok {
		return t, nil
	}
	return TopicEventUnknown, fmt.Errorf("unknown topic name: %q", name)
}

// ParseTopics converts a slice of config strings to a slice of Topics.
func ParseTopics(names []string) ([]Topic, error) {
	topics := make([]Topic, 0, len(names))
	for _, name := range names {
		t, err := ParseTopic(name)
		if err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, nil
}
