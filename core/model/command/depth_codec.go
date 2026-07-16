package command

import "github.com/BullionBear/seq/core/model/codec"

func (r ReqDepthSnapshot) GetBufferLength() int    { return codec.Size[ReqDepthSnapshot]() }
func (r ReqDepthSnapshot) Encode(buf []byte) error { return codec.Encode(buf, &r) }
func NewReqDepthSnapshotFromBytes(buf []byte) (ReqDepthSnapshot, error) {
	return codec.Decode[ReqDepthSnapshot](buf)
}
