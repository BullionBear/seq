package risk

import (
	"github.com/BullionBear/seq/core/logger"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages risk calculations and limits.
// This is a stub implementation for future development.
type Engine struct {
	// Future: risk limits, position tracking, margin calculations
}

// NewEngine creates a new risk engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Init initializes the risk engine.
func (e *Engine) Init() {
	log().Debug().Msg("RiskEngine initialized (stub)")
}

// Start starts the risk engine.
func (e *Engine) Start() {
	log().Debug().Msg("RiskEngine started (stub)")
}

// Stop stops the risk engine.
func (e *Engine) Stop() {
	log().Debug().Msg("RiskEngine stopped (stub)")
}
