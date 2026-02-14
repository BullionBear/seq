package engine

import (
	"time"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
)

// Engine is the common interface for all engines in the system.
// It defines lifecycle methods and state notification capabilities.
type Engine interface {
	// Type returns the engine type identifier
	Type() common.EngineType

	// Lifecycle methods
	Init()
	Start()
	Stop()

	// Execute methods
	Execute(cmd msgbus.Command, bus *msgbus.MsgBus)

	// State notification methods
	NotifyReady()
	NotifyStop()
	NotifyFinished()
	NotifyAbnormal(errorCode int)
}

// EngineBase provides common functionality for all engines.
// Embed this in concrete engine implementations to get state notification support.
type EngineBase struct {
	engineType common.EngineType
	notifier   *msgbus.StateNotifier
}

// NewEngineBase creates a new EngineBase with the given engine type.
// The notifier should be set via SetNotifier before calling notification methods.
func NewEngineBase(engineType common.EngineType) EngineBase {
	return EngineBase{
		engineType: engineType,
	}
}

// Type returns the engine type identifier.
func (e *EngineBase) Type() common.EngineType {
	return e.engineType
}

// Process a command by dispatching to typed callbacks.
// Override the specific typed callbacks (ProcessOrderSubmitCmd, ProcessOrderCancelCmd, etc.)
// in your engine implementation.
func (e *EngineBase) Process(cmd msgbus.Command, bus *msgbus.MsgBus) {
	// Default no-op - concrete engines should override
}

// SetNotifier sets the StateNotifier for this engine.
// This must be called before any notification methods are used.
func (e *EngineBase) SetNotifier(notifier *msgbus.StateNotifier) {
	e.notifier = notifier
}

// Notifier returns the StateNotifier for this engine.
func (e *EngineBase) Notifier() *msgbus.StateNotifier {
	return e.notifier
}

// NotifyReady publishes a ReadyEvent indicating this engine is ready.
func (e *EngineBase) NotifyReady() {
	if e.notifier != nil {
		e.notifier.NotifyReady(e.engineType, uint64(time.Now().UnixMilli()))
	}
}

// NotifyStop publishes a StopEvent indicating this engine is stopping.
func (e *EngineBase) NotifyStop() {
	if e.notifier != nil {
		e.notifier.NotifyStop(e.engineType, uint64(time.Now().UnixMilli()))
	}
}

// NotifyFinished publishes a FinishedEvent indicating this engine has finished.
func (e *EngineBase) NotifyFinished() {
	if e.notifier != nil {
		e.notifier.NotifyFinished(e.engineType, uint64(time.Now().UnixMilli()))
	}
}

// NotifyAbnormal publishes an AbnormalEvent indicating this engine encountered an error.
func (e *EngineBase) NotifyAbnormal(errorCode int) {
	if e.notifier != nil {
		e.notifier.NotifyAbnormal(e.engineType, errorCode, uint64(time.Now().UnixMilli()))
	}
}
