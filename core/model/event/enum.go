package event

type DataType int

const (
	// Market Data
	DataTypeDepthSnapshot DataType = iota
	DataTypeDepthUpdate
	DataTypeTick
	// Execution Data
	DataTypeOrderUpdate
	DataTypeFill
	// Reconciliation Data
	DataTypeBalanceSnapshot
	DataTypeBalanceUpdate
)
