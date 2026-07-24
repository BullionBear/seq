package risk

import (
	"fmt"
	"time"

	"github.com/BullionBear/seq/core/actor"
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

type guardEntry struct {
	name  string
	guard Guard
}

// Engine manages risk calculations and limits.
// It constructs actors from config via the factory registry; actors that also
// implement Guard are collected as the ordered pre-trade check chain.
type Engine struct {
	engine.EngineBase
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	cache   *cache.Cache
	actors  []actor.Actor
	guards  []guardEntry
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

// Init initializes the risk engine: constructs actors from config, collects
// Guards, and registers command handlers. Unknown actor types fail closed.
func (e *Engine) Init(config Config) error {
	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			return fmt.Errorf("risk: actor %q: %w", entry.Name, err)
		}

		a := factory(e.catalog, e.msgBus, e.cache)
		actor.ApplyName(a, entry.Name)
		a.OnInit(entry.Config)

		// core/msgbus: nil/empty topics mean "subscribe to all".
		// Stateless guards must not register, or they receive every event.
		// RegisterIn injects Clock; when skipping registration, inject explicitly.
		if len(a.SubscribedTypes()) > 0 {
			actor.RegisterIn(e.msgBus, a, msgbus.PhaseOf(e.Type()))
		} else {
			actor.InjectClock(e.msgBus, a)
		}
		e.actors = append(e.actors, a)

		g, isGuard := a.(Guard)
		if isGuard {
			e.guards = append(e.guards, guardEntry{name: a.Name(), guard: g})
		}

		log().Info().
			Str("type", entry.Type).
			Str("name", a.Name()).
			Bool("guard", isGuard).
			Msg("RiskEngine: actor initialized")
	}

	for _, cmdType := range e.handledCommandTypes() {
		e.msgBus.RegisterCommand(cmdType, func(cmd msgbus.Command) { e.Execute(cmd, e.msgBus) })
	}
	log().Info().Msg("RiskEngine initialized")
	return nil
}

// Start starts the risk engine and all its actors.
func (e *Engine) Start() {
	for _, a := range e.actors {
		a.OnStart()
	}
	log().Info().Msg("RiskEngine started")
	e.NotifyReady()
}

// Stop stops the risk engine and all its actors.
func (e *Engine) Stop() {
	for _, a := range e.actors {
		a.OnStop()
	}
	log().Info().Msg("RiskEngine stopped")
	e.NotifyStop()
}

// handledCommandTypes returns the command types this engine processes.
func (e *Engine) handledCommandTypes() []command.CommandType {
	return []command.CommandType{
		command.CommandTypeOrderRiskCheck,
	}
}

// Execute routes commands to the appropriate handler.
func (e *Engine) Execute(cmd msgbus.Command, bus *msgbus.MsgBus) {
	switch cmd.Ref.CommandType {
	case command.CommandTypeOrderRiskCheck:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		orderCmd, err := command.NewRiskCheckFromBytes(buf)
		if err != nil {
			log().Error().Err(err).Msg("RiskEngine: failed to decode command")
			return
		}
		e.execOrderRiskCheck(orderCmd)
	}
}

func (e *Engine) execOrderRiskCheck(cmd command.RiskCheck) {
	if name, err := e.riskCheck(cmd); err != nil {
		msg := err.Error()
		if name != "" {
			msg = name + ": " + msg
		}
		log().Error().Err(err).Str("guard", name).Msg("RiskEngine: Order risk check failed")
		ev := event.OrderRiskInvalid{
			ClientOrderID: cmd.ClientOrderID,
			AccountID:     cmd.AccountID,
			ErrorCode:     CodeOf(err),
			Msg:           msg,
		}
		ref, buf, ok := e.msgBus.Allocate(event.TopicEventOrderRiskInvalid, uint64(ev.GetBufferLength()))
		if !ok {
			return
		}
		if err := ev.Encode(buf); err != nil {
			e.msgBus.Cancel(ref)
			return
		}
		e.msgBus.Publish(ref)
		return
	}

	now := cmd.Timestamp
	if now == 0 {
		now = uint64(time.Now().UnixNano())
	}
	ev := event.OrderNew{
		AccountID:     cmd.AccountID,
		ClientOrderID: cmd.ClientOrderID,
		OrderID:       -1,
		SymbolID:      cmd.SymbolID,
		Side:          cmd.Side,
		OrderType:     cmd.OrderType,
		TimeInForce:   cmd.TimeInForce,
		Quantity:      cmd.Quantity,
		Price:         cmd.Price,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	ref, buf, ok := e.msgBus.Allocate(event.TopicEventOrderNew, uint64(ev.GetBufferLength()))
	if !ok {
		return
	}
	if err := ev.Encode(buf); err != nil {
		e.msgBus.Cancel(ref)
		return
	}
	e.msgBus.Publish(ref)
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
	cmdRef, cmdBuf := e.msgBus.AllocateCmd(command.CommandTypeOrderSubmit, uint64(submitCmd.GetBufferLength()))
	submitCmd.Encode(cmdBuf)
	e.msgBus.Send(cmdRef)
}

func (e *Engine) riskCheck(cmd command.RiskCheck) (string, error) {
	for _, ge := range e.guards {
		if err := ge.guard.Check(cmd); err != nil {
			return ge.name, err
		}
	}
	return "", nil
}
