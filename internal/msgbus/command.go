package msgbus

import "github.com/BullionBear/seq/core/model/command"

// CommandRef is a reference to command data stored in the command arena.
// It contains the command type, the offset in the arena, and the data length.
type CommandRef struct {
	CommandType command.CommandType
	Index       uint64 // offset in command arena
	Length      uint64 // size of data in bytes
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
