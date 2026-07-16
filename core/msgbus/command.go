package msgbus

import (
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/command"
)

// CommandRef is a reference to command data stored in the command arena.
// CommandRefs are created by MsgBus.AllocateCmd, which also records the
// arena reservation so the space can be returned after processing.
type CommandRef struct {
	CommandType command.CommandType
	Index       uint64 // offset in command arena
	Length      uint64 // size of data in bytes

	// Arena reservation range (monotonic), set by AllocateCmd.
	resStart uint64
	resEnd   uint64
}

// reservation reconstructs the arena reservation for release.
func (r CommandRef) reservation() mem.Reservation {
	return mem.Reservation{Start: r.resStart, End: r.resEnd, Offset: r.Index}
}

// Command wraps a CommandRef with metadata.
// Commands are point-to-point: each command topic maps to exactly one processor.
type Command struct {
	Ref       CommandRef
	CommandID uint64
	CreatedAt uint64
}

// CommandProcessor is a function type for processing commands.
// Each command topic maps to exactly one CommandProcessor (point-to-point).
type CommandProcessor func(cmd Command)
