package risk

import (
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

// Ensure Engine implements the Engine interface
var _ engine.Engine = (*Engine)(nil)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages risk calculations and limits.
// This is a stub implementation for future development.
type Engine struct {
	// Future: risk limits, position tracking, margin calculations
	engine.EngineBase
}

// NewEngine creates a new risk engine.
func NewEngine() *Engine {
	return &Engine{
		EngineBase: engine.NewEngineBase(common.EngineRisk),
	}
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

// Execute executes a command.
func (e *Engine) Execute(cmd msgbus.Command, bus *msgbus.MsgBus) {
	log().Debug().Msg("RiskEngine executing command (stub)")
	switch cmd.Ref.CommandType {
	case command.CommandTypeOrderRiskCheck:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		orderCmd := command.NewOrderRiskCheckFromBytes(buf)
		e.handleOrderRiskCheck(orderCmd)
	}
}
