package execution

import (
	"github.com/BullionBear/seq/core/logger"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages order execution and routing.
// This is a stub implementation for future development.
type Engine struct {
	// Future: order management, execution routing, fill tracking
}

// NewEngine creates a new execution engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Init initializes the execution engine.
func (e *Engine) Init() {
	log().Debug().Msg("ExecutionEngine initialized (stub)")
}

// Start starts the execution engine.
func (e *Engine) Start() {
	log().Debug().Msg("ExecutionEngine started (stub)")
}

// Stop stops the execution engine.
func (e *Engine) Stop() {
	log().Debug().Msg("ExecutionEngine stopped (stub)")
}
