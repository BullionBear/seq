package command

type CommandType int

const (
	CommandTypeUnknown CommandType = iota
	CommandTypeOrderSubmit
	CommandTypeOrderCancel
	CommandTypeCancelAll

	CommandTypeReqBalanceSnapshot
)
