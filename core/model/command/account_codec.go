package command

import "unsafe"

const sizeOfQryBalanceSnapshot = int(unsafe.Sizeof(QryBalanceSnapshot{}))

func (q QryBalanceSnapshot) GetBufferLength() int { return sizeOfQryBalanceSnapshot }

func (q QryBalanceSnapshot) Encode(buf []byte) error {
	if len(buf) < sizeOfQryBalanceSnapshot {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfQryBalanceSnapshot]byte)(unsafe.Pointer(&q))[:]
	copy(buf, data)
	return nil
}

func NewQryBalanceSnapshotFromBytes(buf []byte) QryBalanceSnapshot {
	return *(*QryBalanceSnapshot)(unsafe.Pointer(&buf[0]))
}
