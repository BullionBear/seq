package command

import "unsafe"

const sizeOfReqDepthSnapshot = int(unsafe.Sizeof(ReqDepthSnapshot{}))

func (r ReqDepthSnapshot) GetBufferLength() int { return sizeOfReqDepthSnapshot }

func (r ReqDepthSnapshot) Encode(buf []byte) error {
	if len(buf) < sizeOfReqDepthSnapshot {
		return ErrBufferTooSmall
	}
	data := (*[sizeOfReqDepthSnapshot]byte)(unsafe.Pointer(&r))[:]
	copy(buf, data)
	return nil
}

func NewReqDepthSnapshotFromBytes(buf []byte) ReqDepthSnapshot {
	return *(*ReqDepthSnapshot)(unsafe.Pointer(&buf[0]))
}
