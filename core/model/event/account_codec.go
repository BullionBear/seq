package event

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/codec"
	"github.com/BullionBear/seq/core/model/common"
)

// Wire headers for the variable-size balance events. Header sizes are
// derived from these structs — padding is declared explicitly, not
// hand-computed. Integer fields are written little-endian; the balance
// array follows the header as raw common.Balance images.
type respBalanceSnapshotHeader struct {
	AccountID   uint64
	WalletID    uint64
	BalancesLen uint32
	_           [4]byte // padding, written as zero
}

type balanceUpdateHeader struct {
	AccountID   uint64
	WalletID    uint64
	UpdatedAt   uint64
	BalancesLen uint32
	_           [4]byte // padding, written as zero
}

const (
	BalanceSize = int(unsafe.Sizeof(common.Balance{}))

	// RespBalanceSnapshotHeaderSize is the size of respBalanceSnapshotHeader.
	RespBalanceSnapshotHeaderSize = int(unsafe.Sizeof(respBalanceSnapshotHeader{}))

	// BalanceUpdateHeaderSize is the size of balanceUpdateHeader.
	BalanceUpdateHeaderSize = int(unsafe.Sizeof(balanceUpdateHeader{}))
)

// validBalanceCount checks the header + n*BalanceSize length invariant
// without integer overflow.
func validBalanceCount(bufLen, headerSize int, balancesLen uint32) bool {
	need := uint64(headerSize) + uint64(balancesLen)*uint64(BalanceSize)
	return uint64(bufLen) >= need
}

// balanceSlice aliases n Balances starting at base. Arena reservations are
// 8-byte aligned and both header sizes are multiples of 8, so the cast is
// aligned.
func balanceSlice(buf []byte, base, n int) []common.Balance {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*common.Balance)(unsafe.Pointer(&buf[base])), n)
}

// ============================================================================
// RespBalanceSnapshot
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a RespBalanceSnapshot.
func (r RespBalanceSnapshot) GetBufferLength() int {
	return RespBalanceSnapshotHeaderSize + len(r.Balances)*BalanceSize
}

// Encode writes the RespBalanceSnapshot into buf (layout: respBalanceSnapshotHeader).
func (r RespBalanceSnapshot) Encode(buf []byte) error {
	if len(buf) < r.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(r.AccountID))
	c.PutUint64(uint64(r.WalletID))
	c.PutUint32(uint32(len(r.Balances)))
	c.PutUint32(0) // padding
	for i := range r.Balances {
		codec.Put(&c, &r.Balances[i])
	}
	return c.Err()
}

// NewRespBalanceSnapshotFromBytes interprets buf as a RespBalanceSnapshot.
// The buffer length is validated against the header-declared balance count
// before any read. The Balances slice is a zero-copy view into buf and is
// only valid while buf is (i.e. within the dispatch handler, before the
// event's arena reservation is released).
func NewRespBalanceSnapshotFromBytes(buf []byte) (RespBalanceSnapshot, error) {
	if len(buf) < RespBalanceSnapshotHeaderSize {
		return RespBalanceSnapshot{}, ErrBufferTooSmall
	}
	accountID := int(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(respBalanceSnapshotHeader{}.AccountID):]))
	walletID := int(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(respBalanceSnapshotHeader{}.WalletID):]))
	balancesLen := binary.LittleEndian.Uint32(buf[unsafe.Offsetof(respBalanceSnapshotHeader{}.BalancesLen):])

	if !validBalanceCount(len(buf), RespBalanceSnapshotHeaderSize, balancesLen) {
		return RespBalanceSnapshot{}, ErrInvalidBuffer
	}

	return RespBalanceSnapshot{
		AccountID: accountID,
		WalletID:  walletID,
		Balances:  balanceSlice(buf, RespBalanceSnapshotHeaderSize, int(balancesLen)),
	}, nil
}

// ============================================================================
// BalanceUpdate
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a BalanceUpdate.
func (b BalanceUpdate) GetBufferLength() int {
	return BalanceUpdateHeaderSize + len(b.Balances)*BalanceSize
}

// Encode writes the BalanceUpdate into buf (layout: balanceUpdateHeader).
func (b BalanceUpdate) Encode(buf []byte) error {
	if len(buf) < b.GetBufferLength() {
		return ErrBufferTooSmall
	}
	c := codec.NewCursor(buf)
	c.PutUint64(uint64(b.AccountID))
	c.PutUint64(uint64(b.WalletID))
	c.PutUint64(b.UpdatedAt)
	c.PutUint32(uint32(len(b.Balances)))
	c.PutUint32(0) // padding
	for i := range b.Balances {
		codec.Put(&c, &b.Balances[i])
	}
	return c.Err()
}

// NewBalanceUpdateFromBytes interprets buf as a BalanceUpdate.
// The buffer length is validated against the header-declared balance count
// before any read. The Balances slice is a zero-copy view into buf and is
// only valid while buf is (i.e. within the dispatch handler, before the
// event's arena reservation is released).
func NewBalanceUpdateFromBytes(buf []byte) (BalanceUpdate, error) {
	if len(buf) < BalanceUpdateHeaderSize {
		return BalanceUpdate{}, ErrBufferTooSmall
	}
	accountID := int(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(balanceUpdateHeader{}.AccountID):]))
	walletID := int(binary.LittleEndian.Uint64(buf[unsafe.Offsetof(balanceUpdateHeader{}.WalletID):]))
	updatedAt := binary.LittleEndian.Uint64(buf[unsafe.Offsetof(balanceUpdateHeader{}.UpdatedAt):])
	balancesLen := binary.LittleEndian.Uint32(buf[unsafe.Offsetof(balanceUpdateHeader{}.BalancesLen):])

	if !validBalanceCount(len(buf), BalanceUpdateHeaderSize, balancesLen) {
		return BalanceUpdate{}, ErrInvalidBuffer
	}

	return BalanceUpdate{
		AccountID: accountID,
		WalletID:  walletID,
		Balances:  balanceSlice(buf, BalanceUpdateHeaderSize, int(balancesLen)),
		UpdatedAt: updatedAt,
	}, nil
}
