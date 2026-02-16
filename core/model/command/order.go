package command

import "github.com/BullionBear/seq/core/model/common"

type RiskCheck struct {
	ClientOrderID int
	AccountID     int
	SymbolID      int
	Side          common.Side
	OrderType     common.OrderType
	TimeInForce   common.TimeInForce
	Price         float64
	Quantity      float64
}

func (r RiskCheck) CommandType() CommandType {
	return CommandTypeOrderRiskCheck
}

type SubmitOrder struct {
	ClientOrderID int
	AccountID     int
	SymbolID      int
	Side          common.Side
	OrderType     common.OrderType
	TimeInForce   common.TimeInForce
	Price         float64
	Quantity      float64
}

func (s SubmitOrder) CommandType() CommandType {
	return CommandTypeOrderSubmit
}

type CancelOrder struct {
	AccountID     int
	ClientOrderID int
}

func (c CancelOrder) CommandType() CommandType {
	return CommandTypeOrderCancel
}

type CancelAll struct {
	AccountID int
	SymbolID  int
}

func (c CancelAll) CommandType() CommandType {
	return CommandTypeCancelAll
}
