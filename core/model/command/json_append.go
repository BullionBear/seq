package command

import "github.com/BullionBear/seq/core/model/codec"

// AppendCommandJSON decodes buf for cmdType and appends the typed JSON object.
func AppendCommandJSON(dst []byte, cmdType CommandType, buf []byte) []byte {
	if len(buf) == 0 {
		return append(dst, "null"...)
	}
	switch cmdType {
	case CommandTypeOrderRiskCheck:
		v, err := NewRiskCheckFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeOrderSubmit:
		v, err := NewSubmitOrderFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeOrderCancel:
		v, err := NewCancelOrderFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeCancelAll:
		v, err := NewCancelAllFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeQryBalanceSnapshot:
		v, err := NewQryBalanceSnapshotFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeReqDepthSnapshot:
		v, err := NewReqDepthSnapshotFromBytes(buf)
		if err != nil {
			return appendDecodeErr(dst, err)
		}
		return v.AppendJSON(dst)
	case CommandTypeReqHistoricalKline:
		v, err := NewReqHistoricalKlineFromBytes(buf)
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

func (r RiskCheck) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.ClientOrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.AccountID))
	dst = append(dst, `,"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.SymbolID))
	dst = append(dst, `,"Side":`...)
	dst = codec.AppendJSONInt(dst, int64(r.Side))
	dst = append(dst, `,"OrderType":`...)
	dst = codec.AppendJSONInt(dst, int64(r.OrderType))
	dst = append(dst, `,"TimeInForce":`...)
	dst = codec.AppendJSONInt(dst, int64(r.TimeInForce))
	dst = append(dst, `,"Price":`...)
	dst = codec.AppendJSONFloat(dst, r.Price)
	dst = append(dst, `,"Quantity":`...)
	dst = codec.AppendJSONFloat(dst, r.Quantity)
	dst = append(dst, `,"Timestamp":`...)
	dst = codec.AppendJSONUint(dst, r.Timestamp)
	return append(dst, '}')
}

func (s SubmitOrder) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(s.ClientOrderID))
	dst = append(dst, `,"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(s.AccountID))
	dst = append(dst, `,"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(s.SymbolID))
	dst = append(dst, `,"Side":`...)
	dst = codec.AppendJSONInt(dst, int64(s.Side))
	dst = append(dst, `,"OrderType":`...)
	dst = codec.AppendJSONInt(dst, int64(s.OrderType))
	dst = append(dst, `,"TimeInForce":`...)
	dst = codec.AppendJSONInt(dst, int64(s.TimeInForce))
	dst = append(dst, `,"Price":`...)
	dst = codec.AppendJSONFloat(dst, s.Price)
	dst = append(dst, `,"Quantity":`...)
	dst = codec.AppendJSONFloat(dst, s.Quantity)
	return append(dst, '}')
}

func (c CancelOrder) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(c.AccountID))
	dst = append(dst, `,"ClientOrderID":`...)
	dst = codec.AppendJSONInt(dst, int64(c.ClientOrderID))
	return append(dst, '}')
}

func (c CancelAll) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(c.AccountID))
	dst = append(dst, `,"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(c.SymbolID))
	return append(dst, '}')
}

func (q QryBalanceSnapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"AccountID":`...)
	dst = codec.AppendJSONInt(dst, int64(q.AccountID))
	return append(dst, '}')
}

func (r ReqDepthSnapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.SymbolID))
	return append(dst, '}')
}

func (r ReqHistoricalKline) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"SymbolID":`...)
	dst = codec.AppendJSONInt(dst, int64(r.SymbolID))
	dst = append(dst, `,"Interval":`...)
	dst = codec.AppendJSONInt(dst, int64(r.Interval))
	dst = append(dst, `,"StartTime":`...)
	dst = codec.AppendJSONUint(dst, r.StartTime)
	dst = append(dst, `,"EndTime":`...)
	dst = codec.AppendJSONUint(dst, r.EndTime)
	dst = append(dst, `,"Limit":`...)
	dst = codec.AppendJSONInt(dst, int64(r.Limit))
	return append(dst, '}')
}
