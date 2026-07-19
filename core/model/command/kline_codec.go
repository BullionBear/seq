package command

import "github.com/BullionBear/seq/core/model/codec"

func (r ReqHistoricalKline) GetBufferLength() int    { return codec.Size[ReqHistoricalKline]() }
func (r ReqHistoricalKline) Encode(buf []byte) error { return codec.Encode(buf, &r) }
func NewReqHistoricalKlineFromBytes(buf []byte) (ReqHistoricalKline, error) {
	return codec.Decode[ReqHistoricalKline](buf)
}
