package portfolio

import (
	"github.com/BullionBear/seq/core/logger"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages portfolio state and calculations.
// This is a stub implementation for future development.
type Engine struct {
	// Future: positions, balances, PnL tracking
}

// NewEngine creates a new portfolio engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Init initializes the portfolio engine.
func (e *Engine) Init() {
	log().Debug().Msg("PortfolioEngine initialized (stub)")
}

// Start starts the portfolio engine.
func (e *Engine) Start() {
	log().Debug().Msg("PortfolioEngine started (stub)")
}

// Stop stops the portfolio engine.
func (e *Engine) Stop() {
	log().Debug().Msg("PortfolioEngine stopped (stub)")
}
