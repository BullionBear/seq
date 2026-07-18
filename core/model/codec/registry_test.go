package codec_test

import (
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// fieldGolden pins one field's byte offset within its wire struct.
type fieldGolden struct {
	name   string
	offset uintptr
}

// wireType is one entry in the registry of every memcpy'd wire type.
// size and fields are the golden layout constants: they were generated once
// from the layout in production at the time the format was frozen. If a
// guard test fails against them, the wire format changed — regenerate these
// constants deliberately.
type wireType struct {
	name   string
	zero   any
	size   uintptr
	fields []fieldGolden
}

// wireTypes is the registry of all types encoded with codec.Encode/Decode/Put
// (directly, or as array elements of a variable-size event). New wire types
// MUST be added here before they are published on the bus or logged.
var wireTypes = []wireType{
	{
		name: "common.PriceLevel",
		zero: common.PriceLevel{},
		size: 32,
		fields: []fieldGolden{
			{"Price", 0},
			{"Quantity", 8},
			{"PriceTick", 16},
			{"QuantityTick", 24},
		},
	},
	{
		name: "common.Balance",
		zero: common.Balance{},
		size: 32,
		fields: []fieldGolden{
			{"TokenID", 0},
			{"Available", 8},
			{"Locked", 16},
			{"Total", 24},
		},
	},
	{
		name: "event.Tick",
		zero: event.Tick{},
		size: 40,
		fields: []fieldGolden{
			{"SymbolID", 0},
			{"Timestamp", 8},
			{"Side", 16},
			{"Price", 24},
			{"Qty", 32},
		},
	},
	{
		name: "event.TimeEvent",
		zero: event.TimeEvent{},
		size: 16,
		fields: []fieldGolden{
			{"TimerID", 0},
			{"ScheduledNs", 8},
		},
	},
	{
		name: "event.ReadyEvent",
		zero: event.ReadyEvent{},
		size: 16,
		fields: []fieldGolden{
			{"Source", 0},
			{"Timestamp", 8},
		},
	},
	{
		name: "event.StopEvent",
		zero: event.StopEvent{},
		size: 16,
		fields: []fieldGolden{
			{"Source", 0},
			{"Timestamp", 8},
		},
	},
	{
		name: "event.FinishedEvent",
		zero: event.FinishedEvent{},
		size: 16,
		fields: []fieldGolden{
			{"Source", 0},
			{"Timestamp", 8},
		},
	},
	{
		name: "event.AbnormalEvent",
		zero: event.AbnormalEvent{},
		size: 24,
		fields: []fieldGolden{
			{"Source", 0},
			{"ErrorCode", 8},
			{"Timestamp", 16},
		},
	},
	{
		name: "event.OrderNew",
		zero: event.OrderNew{},
		size: 96,
		fields: []fieldGolden{
			{"AccountID", 0},
			{"ClientOrderID", 8},
			{"OrderID", 16},
			{"SymbolID", 24},
			{"Side", 32},
			{"OrderType", 40},
			{"TimeInForce", 48},
			{"Quantity", 56},
			{"Price", 64},
			{"ExecutedQty", 72},
			{"CreatedAt", 80},
			{"UpdatedAt", 88},
		},
	},
	{
		name: "event.OrderAccepted",
		zero: event.OrderAccepted{},
		size: 32,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"OrderID", 8},
			{"AccountID", 16},
			{"CreatedAt", 24},
		},
	},
	{
		name: "event.OrderPartiallyFilled",
		zero: event.OrderPartiallyFilled{},
		size: 40,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"OrderID", 8},
			{"AccountID", 16},
			{"ExecutedQty", 24},
			{"UpdatedAt", 32},
		},
	},
	{
		name: "event.OrderFilled",
		zero: event.OrderFilled{},
		size: 40,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"OrderID", 8},
			{"AccountID", 16},
			{"ExecutedQty", 24},
			{"UpdatedAt", 32},
		},
	},
	{
		name: "event.OrderCanceled",
		zero: event.OrderCanceled{},
		size: 40,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"OrderID", 8},
			{"AccountID", 16},
			{"ErrorCode", 24},
			{"UpdatedAt", 32},
		},
	},
	{
		name: "event.Execution",
		zero: event.Execution{},
		size: 96,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"OrderID", 8},
			{"AccountID", 16},
			{"SymbolID", 24},
			{"Side", 32},
			{"IsMaker", 40},
			{"FillID", 48},
			{"FilledQty", 56},
			{"FilledPrice", 64},
			{"FeeCcyID", 72},
			{"FeeQty", 80},
			{"FilledAt", 88},
		},
	},
	{
		name: "command.RiskCheck",
		zero: command.RiskCheck{},
		size: 72,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"AccountID", 8},
			{"SymbolID", 16},
			{"Side", 24},
			{"OrderType", 32},
			{"TimeInForce", 40},
			{"Price", 48},
			{"Quantity", 56},
			{"Timestamp", 64},
		},
	},
	{
		name: "command.SubmitOrder",
		zero: command.SubmitOrder{},
		size: 64,
		fields: []fieldGolden{
			{"ClientOrderID", 0},
			{"AccountID", 8},
			{"SymbolID", 16},
			{"Side", 24},
			{"OrderType", 32},
			{"TimeInForce", 40},
			{"Price", 48},
			{"Quantity", 56},
		},
	},
	{
		name: "command.CancelOrder",
		zero: command.CancelOrder{},
		size: 16,
		fields: []fieldGolden{
			{"AccountID", 0},
			{"ClientOrderID", 8},
		},
	},
	{
		name: "command.CancelAll",
		zero: command.CancelAll{},
		size: 16,
		fields: []fieldGolden{
			{"AccountID", 0},
			{"SymbolID", 8},
		},
	},
	{
		name: "command.QryBalanceSnapshot",
		zero: command.QryBalanceSnapshot{},
		size: 8,
		fields: []fieldGolden{
			{"AccountID", 0},
		},
	},
	{
		name: "command.ReqDepthSnapshot",
		zero: command.ReqDepthSnapshot{},
		size: 8,
		fields: []fieldGolden{
			{"SymbolID", 0},
		},
	},
}
