package command

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
