package command

import "fmt"

type CommandType int

const (
	// Order Commands
	CommandTypeUnknown CommandType = iota
	CommandTypeOrderSubmit
	CommandTypeOrderCancel
	CommandTypeCancelAll

	// Account Commands
	CommandTypeQryBalanceSnapshot

	// Data Commands
	CommandTypeReqDepthSnapshot
)

func (t CommandType) String() string {
	switch t {
	case CommandTypeUnknown:
		return "Unknown"
	case CommandTypeOrderSubmit:
		return "OrderSubmit"
	case CommandTypeOrderCancel:
		return "OrderCancel"
	case CommandTypeCancelAll:
		return "CancelAll"
	case CommandTypeQryBalanceSnapshot:
		return "QryBalanceSnapshot"
	case CommandTypeReqDepthSnapshot:
		return "ReqDepthSnapshot"
	default:
		return fmt.Sprintf("Undefined(%d)", int(t))
	}
}
