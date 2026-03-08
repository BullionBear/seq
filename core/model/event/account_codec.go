package event

import (
	"encoding/binary"
	"unsafe"

	"github.com/BullionBear/seq/core/model/common"
)

const (
	BalanceSize = int(unsafe.Sizeof(common.Balance{}))

	// RespBalanceSnapshotHeaderSize is the header: AccountID(8) + WalletID(8) + BalancesLen(4) + padding(4) = 24 bytes
	RespBalanceSnapshotHeaderSize = 24

	// BalanceUpdateHeaderSize is the header: AccountID(8) + WalletID(8) + UpdatedAt(8) + BalancesLen(4) + padding(4) = 32 bytes
	BalanceUpdateHeaderSize = 32
)

// ============================================================================
// RespBalanceSnapshot
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a RespBalanceSnapshot.
func (r RespBalanceSnapshot) GetBufferLength() int {
	return RespBalanceSnapshotHeaderSize + len(r.Balances)*BalanceSize
}

// Encode writes the RespBalanceSnapshot into buf.
// Layout: [AccountID(8)][WalletID(8)][BalancesLen(4)][Padding(4)][Balances...]
func (r RespBalanceSnapshot) Encode(buf []byte) error {
	needed := r.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	balancesLen := uint32(len(r.Balances))
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.AccountID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(r.WalletID))
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], balancesLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0) // padding
	pos += 4

	for i := range r.Balances {
		balanceBytes := (*[BalanceSize]byte)(unsafe.Pointer(&r.Balances[i]))[:]
		copy(buf[pos:], balanceBytes)
		pos += BalanceSize
	}
	return nil
}

// NewRespBalanceSnapshotFromBytes interprets buf as a RespBalanceSnapshot. Zero-copy for slices.
func NewRespBalanceSnapshotFromBytes(buf []byte) RespBalanceSnapshot {
	pos := 0

	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	walletID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	balancesLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4
	pos += 4 // skip padding

	var balances []common.Balance
	if balancesLen > 0 {
		balances = unsafe.Slice((*common.Balance)(unsafe.Pointer(&buf[pos])), balancesLen)
	}

	return RespBalanceSnapshot{
		AccountID: accountID,
		WalletID:  walletID,
		Balances:  balances,
	}
}

// ============================================================================
// BalanceUpdate
// ============================================================================

// GetBufferLength returns the number of bytes needed to encode a BalanceUpdate.
func (b BalanceUpdate) GetBufferLength() int {
	return BalanceUpdateHeaderSize + len(b.Balances)*BalanceSize
}

// Encode writes the BalanceUpdate into buf.
// Layout: [AccountID(8)][WalletID(8)][UpdatedAt(8)][BalancesLen(4)][Padding(4)][Balances...]
func (b BalanceUpdate) Encode(buf []byte) error {
	needed := b.GetBufferLength()
	if len(buf) < needed {
		return ErrBufferTooSmall
	}
	balancesLen := uint32(len(b.Balances))
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], uint64(b.AccountID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], uint64(b.WalletID))
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], b.UpdatedAt)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], balancesLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], 0) // padding
	pos += 4

	for i := range b.Balances {
		balanceBytes := (*[BalanceSize]byte)(unsafe.Pointer(&b.Balances[i]))[:]
		copy(buf[pos:], balanceBytes)
		pos += BalanceSize
	}
	return nil
}

// NewBalanceUpdateFromBytes interprets buf as a BalanceUpdate. Zero-copy for slices.
func NewBalanceUpdateFromBytes(buf []byte) BalanceUpdate {
	pos := 0

	accountID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	walletID := int(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8
	updatedAt := binary.LittleEndian.Uint64(buf[pos:])
	pos += 8
	balancesLen := binary.LittleEndian.Uint32(buf[pos:])
	pos += 4
	pos += 4 // skip padding

	var balances []common.Balance
	if balancesLen > 0 {
		balances = unsafe.Slice((*common.Balance)(unsafe.Pointer(&buf[pos])), balancesLen)
	}

	return BalanceUpdate{
		AccountID: accountID,
		WalletID:  walletID,
		Balances:  balances,
		UpdatedAt: updatedAt,
	}
}
