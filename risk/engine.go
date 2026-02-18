package risk

import (
	"errors"
	"time"

	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
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
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	cache   *cache.Cache
}

// NewEngine creates a new risk engine.
func NewEngine(cat *catalog.Catalog, msgBus *msgbus.MsgBus, c *cache.Cache) *Engine {
	return &Engine{
		EngineBase: engine.NewEngineBase(common.EngineRisk),
		catalog:    cat,
		msgBus:     msgBus,
		cache:      c,
	}
}

// Init initializes the risk engine.
func (e *Engine) Init() {
	log().Debug().Msg("RiskEngine initialized (stub)")
	for _, cmdType := range e.handledCommandTypes() {
		e.msgBus.RegisterCommand(cmdType, func(cmd msgbus.Command) { e.Execute(cmd, e.msgBus) })
	}
	log().Debug().Msg("RiskEngine initialized")
}

// Start starts the risk engine.
func (e *Engine) Start() {
	log().Debug().Msg("RiskEngine started (stub)")
	e.NotifyReady()
}

// Stop stops the risk engine.
func (e *Engine) Stop() {
	log().Debug().Msg("RiskEngine stopped (stub)")
	e.NotifyStop()
}

// handledCommandTypes returns the command types this engine processes.
func (e *Engine) handledCommandTypes() []command.CommandType {
	return []command.CommandType{
		command.CommandTypeOrderRiskCheck,
	}
}

// Execute executes a command.
func (e *Engine) Execute(cmd msgbus.Command, bus *msgbus.MsgBus) {
	log().Debug().Msg("RiskEngine executing command (stub)")
	switch cmd.Ref.CommandType {
	case command.CommandTypeOrderRiskCheck:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		orderCmd := command.NewRiskCheckFromBytes(buf)
		e.execOrderRiskCheck(orderCmd)
	}
}

func (e *Engine) execOrderRiskCheck(cmd command.RiskCheck) {
	log().Debug().Msg("RiskEngine executing order risk check (stub)")
	if err := e.riskCheck(cmd); err != nil {
		log().Error().Err(err).Msg("RiskEngine: Order risk check failed")
		ev := event.OrderRiskInvalid{
			ClientOrderID: cmd.ClientOrderID,
			AccountID:     cmd.AccountID,
			ErrorCode:     -1,
			Msg:           err.Error(),
		}
		offset, buf := e.msgBus.Allocate(uint64(ev.GetBufferLength()))
		ev.Encode(buf)
		e.msgBus.Publish(msgbus.EventRef{
			Topic:  event.TopicEventOrderRiskInvalid,
			Index:  offset,
			Length: uint64(ev.GetBufferLength()),
		})
		return
	}

	ev := event.OrderNew{
		ClientOrderID: cmd.ClientOrderID,
		OrderID:       -1,
		AccountID:     cmd.AccountID,
		CreatedAt:     uint64(time.Now().UnixNano()),
	}
	offset, buf := e.msgBus.Allocate(uint64(ev.GetBufferLength()))
	ev.Encode(buf)
	e.msgBus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventOrderNew,
		Index:  offset,
		Length: uint64(ev.GetBufferLength()),
	})
	submitCmd := command.SubmitOrder{
		ClientOrderID: cmd.ClientOrderID,
		AccountID:     cmd.AccountID,
		SymbolID:      cmd.SymbolID,
		Side:          cmd.Side,
		OrderType:     cmd.OrderType,
		TimeInForce:   cmd.TimeInForce,
		Price:         cmd.Price,
		Quantity:      cmd.Quantity,
	}
	offset, buf = e.msgBus.AllocateCmd(uint64(submitCmd.GetBufferLength()))
	submitCmd.Encode(buf)
	e.msgBus.Send(msgbus.CommandRef{
		CommandType: command.CommandTypeOrderSubmit,
		Index:       offset,
		Length:      uint64(submitCmd.GetBufferLength()),
	})
}

func (e *Engine) riskCheck(cmd command.RiskCheck) error {
	if cmd.ClientOrderID < 0 {
		return errors.New("client order ID is negative")
	}
	return nil
}
