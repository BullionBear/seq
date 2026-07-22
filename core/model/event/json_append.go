package event

import (
	"github.com/BullionBear/seq/core/model/codec"
	"github.com/BullionBear/seq/core/model/common"
)

// AppendEventJSON decodes buf for topic and appends the typed JSON object.
// On decode failure it appends {"decode_error":"..."}. Unknown topics append
// {"raw_len":N}. Empty buf appends null.
func AppendEventJSON(dst []byte, topic Topic, buf []byte) []byte {
	if len(buf) == 0 {
		return append(dst, "null"...)
	}
	switch topic {
	case TopicEventAbnormal:
		v, err := NewAbnormalEventFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventReady:
		v, err := NewReadyEventFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventStop:
		v, err := NewStopEventFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventFinished:
		v, err := NewFinishedEventFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventDepthSnapshot:
		v, err := NewDepthSnapshotFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventRespDepthSnapshot:
		v, err := NewRespDepthSnapshotFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventDepthUpdate:
		v, err := NewDepthUpdateFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventTick:
		v, err := NewTickFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventKline:
		v, err := NewKlineFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventRespHistoricalKline:
		v, err := NewRespHistoricalKlineFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventTimer:
		v, err := NewTimeEventFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderNew:
		v, err := NewOrderNewFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderUnknownStatus:
		v, err := NewOrderUnknownStatusFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderError:
		v, err := NewOrderErrorFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderRiskInvalid:
		v, err := NewOrderRiskInvalidFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderAccepted:
		v, err := NewOrderAcceptedFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderPartialFill:
		v, err := NewOrderPartiallyFilledFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderFilled:
		v, err := NewOrderFilledFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventExecution:
		v, err := NewExecutionFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderCanceled:
		v, err := NewOrderCanceledFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventOrderRejected:
		v, err := NewOrderRejectedFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventRespBalanceSnapshot:
		v, err := NewRespBalanceSnapshotFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case TopicEventBalanceUpdate:
		v, err := NewBalanceUpdateFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	default:
		dst = append(dst, `{"raw_len":`...)
		dst = codec.AppendJSONInt(dst, int64(len(buf)))
		return append(dst, '}')
	}
}

func appendDecodeErr(dst []byte, err error) []byte {
	dst = append(dst, `{"decode_error":`...)
	dst = codec.AppendJSONString(dst, err.Error())
	return append(dst, '}')
}

// --- per-type AppendJSON (field names match encoding/json defaults) ---

func (t Tick) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(t.SymbolID))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, t.Timestamp)
	dst = append(dst, `,"Side":`...)
	dst = codec.AppendJSONInt(dst, int64(t.Side))
	dst = append(dst, `,"Price":`...)
	dst = codec.AppendJSONFloat(dst, t.Price)
	dst = append(dst, `,"Qty":`...)
	dst = codec.AppendJSONFloat(dst, t.Qty)
	return append(dst, '}')
}

func (t TimeEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"TimerID":`...)
	dst = codec.AppendJSONUint(dst, t.TimerID)
	dst = append(dst, `,"ScheduledNs":`...)
	dst = codec.AppendJSONUint(dst, t.ScheduledNs)
	return append(dst, '}')
}

func (r ReadyEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"Source":`...)
	dst = codec.AppendJSONInt(dst, int64(r.Source))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, r.Timestamp)
	return append(dst, '}')
}

func (s StopEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"Source":`...)
	dst = codec.AppendJSONInt(dst, int64(s.Source))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, s.Timestamp)
	return append(dst, '}')
}

func (f FinishedEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"Source":`...)
	dst = codec.AppendJSONInt(dst, int64(f.Source))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, f.Timestamp)
	return append(dst, '}')
}

func (a AbnormalEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"Source":`...)
	dst = codec.AppendJSONInt(dst, int64(a.Source))
	dst = append(dst, `,"ErrorCode":`...)
	dst = codec.AppendJSONInt(dst, int64(a.ErrorCode))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, a.Timestamp)
	return append(dst, '}')
}

func (d DepthSnapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.SymbolID))
	dst = append(dst, `,"DepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.DepthID))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, d.Timestamp)
	dst = append(dst, `,"Asks":`...)
	dst = common.AppendPriceLevelsJSON(dst, d.Asks)
	dst = append(dst, `,"Bids":`...)
	dst = common.AppendPriceLevelsJSON(dst, d.Bids)
	return append(dst, '}')
}

func (d DepthUpdate) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.SymbolID))
	dst = append(dst, `,"PreviousDepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.PreviousDepthID))
	dst = append(dst, `,"DepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.DepthID))
	dst = append(dst, `,"CurrentDepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.CurrentDepthID))
	dst = append(dst, `,"NextDepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(d.NextDepthID))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, d.Timestamp)
	dst = append(dst, `,"Asks":`...)
	dst = common.AppendPriceLevelsJSON(dst, d.Asks)
	dst = append(dst, `,"Bids":`...)
	dst = common.AppendPriceLevelsJSON(dst, d.Bids)
	return append(dst, '}')
}

func (r RespDepthSnapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.SymbolID))
	dst = append(dst, `,"DepthID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.DepthID))
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, r.Timestamp)
	dst = append(dst, `,"AskLength":`...)
	dst = codec.AppendJSONInt(dst, int64(r.AskLength))
	dst = append(dst, `,"BidLength":`...)
	dst = codec.AppendJSONInt(dst, int64(r.BidLength))
	dst = append(dst, `,"Asks":`...)
	dst = common.AppendPriceLevelsJSON(dst, r.Asks)
	dst = append(dst, `,"Bids":`...)
	dst = common.AppendPriceLevelsJSON(dst, r.Bids)
	return append(dst, '}')
}

func (k Kline) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(k.SymbolID))
	dst = append(dst, `,"Interval":`...)
	dst = codec.AppendJSONInt(dst, int64(k.Interval))
	dst = append(dst, `,"StartTime":`...)
	dst = codec.AppendJSONUint(dst, k.StartTime)
	dst = append(dst, `,"EndTime":`...)
	dst = codec.AppendJSONUint(dst, k.EndTime)
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, k.Timestamp)
	dst = append(dst, `,"Open":`...)
	dst = codec.AppendJSONFloat(dst, k.Open)
	dst = append(dst, `,"High":`...)
	dst = codec.AppendJSONFloat(dst, k.High)
	dst = append(dst, `,"Low":`...)
	dst = codec.AppendJSONFloat(dst, k.Low)
	dst = append(dst, `,"Close":`...)
	dst = codec.AppendJSONFloat(dst, k.Close)
	dst = append(dst, `,"Volume":`...)
	dst = codec.AppendJSONFloat(dst, k.Volume)
	dst = append(dst, `,"QuoteVolume":`...)
	dst = codec.AppendJSONFloat(dst, k.QuoteVolume)
	dst = append(dst, `,"TradeCount":`...)
	dst = codec.AppendJSONInt(dst, int64(k.TradeCount))
	dst = append(dst, `,"Closed":`...)
	dst = codec.AppendJSONBool(dst, k.Closed)
	return append(dst, '}')
}

func (b KlineBar) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"StartTime":`...)
	dst = codec.AppendJSONUint(dst, b.StartTime)
	dst = append(dst, `,"EndTime":`...)
	dst = codec.AppendJSONUint(dst, b.EndTime)
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, b.Timestamp)
	dst = append(dst, `,"Open":`...)
	dst = codec.AppendJSONFloat(dst, b.Open)
	dst = append(dst, `,"High":`...)
	dst = codec.AppendJSONFloat(dst, b.High)
	dst = append(dst, `,"Low":`...)
	dst = codec.AppendJSONFloat(dst, b.Low)
	dst = append(dst, `,"Close":`...)
	dst = codec.AppendJSONFloat(dst, b.Close)
	dst = append(dst, `,"Volume":`...)
	dst = codec.AppendJSONFloat(dst, b.Volume)
	dst = append(dst, `,"QuoteVolume":`...)
	dst = codec.AppendJSONFloat(dst, b.QuoteVolume)
	dst = append(dst, `,"TradeCount":`...)
	dst = codec.AppendJSONInt(dst, int64(b.TradeCount))
	dst = append(dst, `,"Closed":`...)
	dst = codec.AppendJSONBool(dst, b.Closed)
	return append(dst, '}')
}

func (r RespHistoricalKline) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.SymbolID))
	dst = append(dst, `,"Interval":`...)
	dst = codec.AppendJSONInt(dst, int64(r.Interval))
	dst = append(dst, `,"Bars":`...)
	if r.Bars == nil {
		dst = append(dst, "null"...)
	} else {
		dst = append(dst, '[')
		for i, b := range r.Bars {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = b.AppendJSON(dst)
		}
		dst = append(dst, ']')
	}
	return append(dst, '}')
}

func (o OrderNew) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.SymbolID))
	dst = append(dst, `,"Side":`...)
	dst = codec.AppendJSONInt(dst, int64(o.Side))
	dst = append(dst, `,"OrderType":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderType))
	dst = append(dst, `,"TimeInForce":`...)
	dst = codec.AppendJSONInt(dst, int64(o.TimeInForce))
	dst = append(dst, `,"Quantity":`...)
	dst = codec.AppendJSONFloat(dst, o.Quantity)
	dst = append(dst, `,"Price":`...)
	dst = codec.AppendJSONFloat(dst, o.Price)
	dst = append(dst, `,"ExecutedQty":`...)
	dst = codec.AppendJSONFloat(dst, o.ExecutedQty)
	dst = append(dst, `,"CreatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.CreatedAt)
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.UpdatedAt)
	return append(dst, '}')
}

func (o OrderAccepted) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"CreatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.CreatedAt)
	return append(dst, '}')
}

func (o OrderPartiallyFilled) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ExecutedQty":`...)
	dst = codec.AppendJSONFloat(dst, o.ExecutedQty)
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.UpdatedAt)
	return append(dst, '}')
}

func (o OrderFilled) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ExecutedQty":`...)
	dst = codec.AppendJSONFloat(dst, o.ExecutedQty)
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.UpdatedAt)
	return append(dst, '}')
}

func (o OrderCanceled) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ErrorCode":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ErrorCode))
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.UpdatedAt)
	return append(dst, '}')
}

func (e Execution) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.AccountID))
	dst = append(dst, `,"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.SymbolID))
	dst = append(dst, `,"Side":`...)
	dst = codec.AppendJSONInt(dst, int64(e.Side))
	dst = append(dst, `,"IsMaker":`...)
	dst = codec.AppendJSONBool(dst, e.IsMaker)
	dst = append(dst, `,"FillID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.FillID))
	dst = append(dst, `,"FilledQty":`...)
	dst = codec.AppendJSONFloat(dst, e.FilledQty)
	dst = append(dst, `,"FilledPrice":`...)
	dst = codec.AppendJSONFloat(dst, e.FilledPrice)
	dst = append(dst, `,"FeeCcyID":`...)
	dst = codec.AppendJSONInt(dst, int64(e.FeeCcyID))
	dst = append(dst, `,"FeeQty":`...)
	dst = codec.AppendJSONFloat(dst, e.FeeQty)
	dst = append(dst, `,"FilledAt":`...)
	dst = codec.AppendJSONUint(dst, e.FilledAt)
	return append(dst, '}')
}

func (o OrderUnknownStatus) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"Msg":`...)
	dst = codec.AppendJSONString(dst, o.Msg)
	return append(dst, '}')
}

func (o OrderError) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ErrorCode":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ErrorCode))
	dst = append(dst, `,"Msg":`...)
	dst = codec.AppendJSONString(dst, o.Msg)
	return append(dst, '}')
}

func (o OrderRejected) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"OrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.OrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ErrorCode":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ErrorCode))
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, o.UpdatedAt)
	dst = append(dst, `,"Msg":`...)
	dst = codec.AppendJSONString(dst, o.Msg)
	return append(dst, '}')
}

func (o OrderRiskInvalid) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ClientOrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(o.AccountID))
	dst = append(dst, `,"ErrorCode":`...)
	dst = codec.AppendJSONInt(dst, int64(o.ErrorCode))
	dst = append(dst, `,"Msg":`...)
	dst = codec.AppendJSONString(dst, o.Msg)
	return append(dst, '}')
}

func (r RespBalanceSnapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.AccountID))
	dst = append(dst, `,"WalletID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.WalletID))
	dst = append(dst, `,"Balances":`...)
	dst = common.AppendBalancesJSON(dst, r.Balances)
	return append(dst, '}')
}

func (b BalanceUpdate) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(b.AccountID))
	dst = append(dst, `,"WalletID":`...)
	dst = codec.AppendJSONInt(dst, int64(b.WalletID))
	dst = append(dst, `,"Balances":`...)
	dst = common.AppendBalancesJSON(dst, b.Balances)
	dst = append(dst, `,"UpdatedAt":`...)
	dst = codec.AppendJSONUint(dst, b.UpdatedAt)
	return append(dst, '}')
}
