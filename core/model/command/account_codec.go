package command

import "github.com/BullionBear/seq/core/model/codec"

func (q QryBalanceSnapshot) GetBufferLength() int    { return codec.Size[QryBalanceSnapshot]() }
func (q QryBalanceSnapshot) Encode(buf []byte) error { return codec.Encode(buf, &q) }
func NewQryBalanceSnapshotFromBytes(buf []byte) (QryBalanceSnapshot, error) {
	return codec.Decode[QryBalanceSnapshot](buf)
}
