package msgbus

import (
	"time"

	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

const (
	// DefaultCommandRingBufferSize is the default size for the command ring buffer.
	// Smaller than events since commands are less frequent.
	DefaultCommandRingBufferSize = 1024

	// DefaultCommandArenaCapacity is the default capacity for the command byte arena (256KB).
	// Command payloads are typically small.
	DefaultCommandArenaCapacity = 256 * 1024
)

// MsgBus is the central message distribution system that supports both
// pub-sub events and point-to-point commands.
//
// Event channel (pub-sub):
//   - Uses MPSC (Multiple Producer Single Consumer) ring buffer
//   - Multiple goroutines can publish events concurrently
//   - Events are fan-out to all matching consumers
//
// Command channel (point-to-point):
//   - Uses SPSC (Single Producer Single Consumer) ring buffer
//   - Commands are produced on the dispatch thread (during event handling)
//   - Each command topic maps to exactly one handler
//   - Commands always have higher priority than events
//
// Thread safety:
//   - Publish/Allocate can be called from multiple goroutines (event channel)
//   - Send/AllocateCmd should only be called from the dispatch thread (command channel)
//   - Dispatch should be called from a single goroutine
type MsgBus struct {
	// Event channel (pub-sub, MPSC)
	eventBus *EventBus

	// Command channel (point-to-point, SPSC)
	nextCommandID uint64
	rbCommand     *mem.SPSCRingBuffer[Command]
	cmdArena      *mem.SimpleByteArena
	cmdHandlers   map[command.CommandType]CommandHandler
}

// NewMsgBus creates a new MsgBus with default capacities.
func NewMsgBus() *MsgBus {
	return &MsgBus{
		eventBus:      NewEventBus(),
		nextCommandID: 0,
		rbCommand:     mem.NewSPSCRingBuffer[Command](DefaultCommandRingBufferSize),
		cmdArena:      mem.NewSimpleByteArena(DefaultCommandArenaCapacity),
		cmdHandlers:   make(map[command.CommandType]CommandHandler),
	}
}

// NewMsgBusWithCapacity creates a new MsgBus with custom event arena capacity.
func NewMsgBusWithCapacity(eventArenaCapacity uint64) *MsgBus {
	return &MsgBus{
		eventBus:      NewEventBusWithCapacity(eventArenaCapacity),
		nextCommandID: 0,
		rbCommand:     mem.NewSPSCRingBuffer[Command](DefaultCommandRingBufferSize),
		cmdArena:      mem.NewSimpleByteArena(DefaultCommandArenaCapacity),
		cmdHandlers:   make(map[command.CommandType]CommandHandler),
	}
}

// =============================================================================
// Event API (pub-sub, MPSC)
// =============================================================================

// Register adds an event consumer to the MsgBus with optional topic filtering.
// If topics is nil or empty, the consumer will receive all event topics.
// Consumers should be registered before calling Dispatch.
func (m *MsgBus) Register(name string, topics []event.Topic, handler EventHandler) {
	m.eventBus.Register(name, topics, handler)
}

// Publish publishes an EventRef to the event ring buffer. The caller should have
// already serialized data into the arena buffer obtained via Allocate.
// This method is thread-safe and can be called from multiple goroutines.
func (m *MsgBus) Publish(ref EventRef) {
	m.eventBus.Publish(ref)
}

// Allocate reserves space in the event arena and returns the offset and a []byte slice
// for the caller to write data into.
func (m *MsgBus) Allocate(size uint64) (offset uint64, buffer []byte) {
	return m.eventBus.Allocate(size)
}

// ReadBuffer returns a []byte slice from the event arena at the given offset/length
// for consumers to deserialize event data from.
func (m *MsgBus) ReadBuffer(offset, length uint64) []byte {
	return m.eventBus.ReadBuffer(offset, length)
}

// Poll reads the next event from the ring buffer and calls the handler.
// Returns true if an event was processed, false if the ring buffer is empty.
// This is a single-consumer convenience method (no topic filtering).
func (m *MsgBus) Poll(handler EventHandler) bool {
	return m.eventBus.Poll(handler)
}

// ConsumerCount returns the number of registered event consumers.
func (m *MsgBus) ConsumerCount() int {
	return m.eventBus.ConsumerCount()
}

// MinSequence returns the minimum sequence across all event consumers.
func (m *MsgBus) MinSequence() uint64 {
	return m.eventBus.MinSequence()
}

// =============================================================================
// Command API (point-to-point, SPSC)
// =============================================================================

// RegisterCommand registers a handler for a specific command topic.
// Each command topic can have at most one handler (point-to-point).
// Panics if a handler is already registered for the given topic.
func (m *MsgBus) RegisterCommand(cmdType command.CommandType, handler CommandHandler) {
	if _, exists := m.cmdHandlers[cmdType]; exists {
		log().Panic().
			Int("cmdType", int(cmdType)).
			Msg("MsgBus: duplicate command handler registration")
	}
	m.cmdHandlers[cmdType] = handler
}

// Send sends a command to the SPSC command ring buffer.
// This should only be called from the dispatch thread (single producer).
func (m *MsgBus) Send(ref CommandRef) {
	now := uint64(time.Now().UnixNano())
	commandID := m.nextCommandID
	m.nextCommandID++
	if !m.rbCommand.Write(Command{
		Ref:       ref,
		CommandID: commandID,
		CreatedAt: now,
	}) {
		log().Error().
			Int("cmdType", int(ref.CommandType)).
			Uint64("commandID", commandID).
			Msg("MsgBus: command ring buffer full, dropping command")
	}
}

// AllocateCmd reserves space in the command arena and returns the offset and a []byte slice
// for the caller to write command data into.
// This should only be called from the dispatch thread (single producer).
func (m *MsgBus) AllocateCmd(size uint64) (offset uint64, buffer []byte) {
	offset = m.cmdArena.Reserve(size)
	buffer = m.cmdArena.GetSlice(offset, size)
	return offset, buffer
}

// ReadCmdBuffer returns a []byte slice from the command arena at the given offset/length
// for command handlers to deserialize command data from.
func (m *MsgBus) ReadCmdBuffer(offset, length uint64) []byte {
	return m.cmdArena.ReadSlice(offset, length)
}

// =============================================================================
// Dispatch (unified, priority-based)
// =============================================================================

// Dispatch processes messages with command priority.
// It first drains ALL pending commands from the SPSC ring buffer,
// then processes one event from the MPSC ring buffer.
// Returns true if any work was done (command or event dispatched).
func (m *MsgBus) Dispatch() bool {
	hasWork := false

	// Phase 1: Drain ALL pending commands (higher priority)
	for {
		cmd, ok := m.rbCommand.Read()
		if !ok {
			break
		}
		hasWork = true
		handler, exists := m.cmdHandlers[cmd.Ref.CommandType]
		if !exists {
			log().Warn().
				Int("cmdType", int(cmd.Ref.CommandType)).
				Uint64("commandID", cmd.CommandID).
				Msg("MsgBus: no handler registered for command type")
			continue
		}
		handler(cmd)
	}

	// Phase 2: Process one event (lower priority)
	if m.eventBus.Dispatch() {
		hasWork = true
	}

	return hasWork
}

// Release updates minSequence for the event channel.
// This indicates the oldest event still being processed.
func (m *MsgBus) Release() {
	m.eventBus.Release()
}

// ReleaseArenas is kept for backward compatibility with the event channel.
func (m *MsgBus) ReleaseArenas() {
	m.eventBus.ReleaseArenas()
}
