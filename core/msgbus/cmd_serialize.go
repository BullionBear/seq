package msgbus

import (
	"unsafe"

	"github.com/BullionBear/seq/core/model/command"
)

// Size constants for command types
const (
	SizeOfOrderSubmitCommand = int(unsafe.Sizeof(command.SubmitOrder{}))
	SizeOfOrderCancelCommand = int(unsafe.Sizeof(command.CancelOrder{}))
	SizeOfCancelAllCommand   = int(unsafe.Sizeof(command.CancelAll{}))
)

// =============================================================================
// OrderSubmitCommand Serialization
// =============================================================================

// OrderSubmitCommandSize returns the size needed to serialize an OrderSubmitCommand.
func OrderSubmitCommandSize() uint64 {
	return uint64(SizeOfOrderSubmitCommand)
}

// SerializeOrderSubmitCommand writes an OrderSubmitCommand to the buffer.
// Returns the number of bytes written.
func SerializeOrderSubmitCommand(buf []byte, cmd *command.SubmitOrder) int {
	data := (*[SizeOfOrderSubmitCommand]byte)(unsafe.Pointer(cmd))[:]
	copy(buf, data)
	return SizeOfOrderSubmitCommand
}

// DeserializeOrderSubmitCommand reads an OrderSubmitCommand from buffer.
func DeserializeOrderSubmitCommand(buf []byte) command.SubmitOrder {
	return *(*command.SubmitOrder)(unsafe.Pointer(&buf[0]))
}

// =============================================================================
// OrderCancelCommand Serialization
// =============================================================================

// OrderCancelCommandSize returns the size needed to serialize an OrderCancelCommand.
func OrderCancelCommandSize() uint64 {
	return uint64(SizeOfOrderCancelCommand)
}

// SerializeOrderCancelCommand writes an OrderCancelCommand to the buffer.
// Returns the number of bytes written.
func SerializeOrderCancelCommand(buf []byte, cmd *command.CancelOrder) int {
	data := (*[SizeOfOrderCancelCommand]byte)(unsafe.Pointer(cmd))[:]
	copy(buf, data)
	return SizeOfOrderCancelCommand
}

// DeserializeOrderCancelCommand reads an OrderCancelCommand from buffer.
func DeserializeOrderCancelCommand(buf []byte) command.CancelOrder {
	return *(*command.CancelOrder)(unsafe.Pointer(&buf[0]))
}

// =============================================================================
// CancelAllCommand Serialization
// =============================================================================

// CancelAllCommandSize returns the size needed to serialize a CancelAllCommand.
func CancelAllCommandSize() uint64 {
	return uint64(SizeOfCancelAllCommand)
}

// SerializeCancelAllCommand writes a CancelAllCommand to the buffer.
// Returns the number of bytes written.
func SerializeCancelAllCommand(buf []byte, cmd *command.CancelAll) int {
	data := (*[SizeOfCancelAllCommand]byte)(unsafe.Pointer(cmd))[:]
	copy(buf, data)
	return SizeOfCancelAllCommand
}

// DeserializeCancelAllCommand reads a CancelAllCommand from buffer.
func DeserializeCancelAllCommand(buf []byte) command.CancelAll {
	return *(*command.CancelAll)(unsafe.Pointer(&buf[0]))
}
