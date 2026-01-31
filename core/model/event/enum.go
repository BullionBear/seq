package event

type DataType int

const (
	// Market Data
	DataTypeDepthSnapshot DataType = iota
	DataTypeReqDepthSnapshot
	DataTypeDepthUpdate
	DataTypeTick
	// Execution Data
	DataTypeOrderUpdate
	DataTypeFill
	// Reconciliation Data
	DataTypeBalanceSnapshot
	DataTypeBalanceUpdate
)
