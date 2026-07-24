package execution

import (
	"context"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

var _ engine.Engine = (*Engine)(nil)

// Engine manages order execution and maintains open order state.
// It constructs actors from config via the factory registry.
type Engine struct {
	engine.EngineBase
	router *adapter.ExecutionRouter
	msgBus *msgbus.MsgBus
	cache  *cache.Cache
	actors []actor.Actor
}

// NewEngine creates a new execution engine
func NewEngine(router *adapter.ExecutionRouter, msgBus *msgbus.MsgBus, cache *cache.Cache) *Engine {
	return &Engine{
		EngineBase: engine.NewEngineBase(common.EngineExecution),
		router:     router,
		msgBus:     msgBus,
		cache:      cache,
	}
}

// handledCommandTypes returns the command types this engine processes.
func (e *Engine) handledCommandTypes() []command.CommandType {
	return []command.CommandType{
		command.CommandTypeOrderSubmit,
		command.CommandTypeOrderCancel,
		command.CommandTypeCancelAll,
	}
}

// Init constructs actors from config, registers them and command processors.
func (e *Engine) Init(config Config) {
	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			log().Error().Err(err).Str("type", entry.Type).Msg("ExecutionEngine: skipping unknown actor type")
			continue
		}

		a := factory(e.msgBus, e.cache)
		actor.ApplyName(a, entry.Name)
		a.OnInit(entry.Config)
		actor.RegisterIn(e.msgBus, a, msgbus.PhaseOf(e.Type()))
		e.actors = append(e.actors, a)

		log().Info().Str("type", entry.Type).Str("name", a.Name()).Msg("ExecutionEngine: actor initialized")
	}

	for _, cmdType := range e.handledCommandTypes() {
		e.msgBus.RegisterCommand(cmdType, func(cmd msgbus.Command) { e.Execute(cmd, e.msgBus) })
	}

	// Prepare subscriptions on execution clients (deferred until Connect)
	for _, acctID := range e.router.AccountIDs() {
		if err := e.router.SubscribeOrderUpdate(acctID); err != nil {
			log().Error().Err(err).Int("accountID", acctID).Msg("ExecutionEngine: Failed to prepare order update subscription")
		}
		if err := e.router.SubscribeFill(acctID); err != nil {
			log().Error().Err(err).Int("accountID", acctID).Msg("ExecutionEngine: Failed to prepare fill subscription")
		}
	}

	log().Info().Msg("ExecutionEngine initialized")
}

// Execute routes commands to the appropriate handler.
func (e *Engine) Execute(cmd msgbus.Command, bus *msgbus.MsgBus) {
	switch cmd.Ref.CommandType {
	case command.CommandTypeOrderSubmit:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		submitCmd, err := command.NewSubmitOrderFromBytes(buf)
		if err != nil {
			log().Error().Err(err).Msg("ExecutionEngine: failed to decode command")
			return
		}
		e.execOrderSubmit(submitCmd)
	case command.CommandTypeOrderCancel:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		cancelCmd, err := command.NewCancelOrderFromBytes(buf)
		if err != nil {
			log().Error().Err(err).Msg("ExecutionEngine: failed to decode command")
			return
		}
		e.execOrderCancel(cancelCmd)
	case command.CommandTypeCancelAll:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		cancelAllCmd, err := command.NewCancelAllFromBytes(buf)
		if err != nil {
			log().Error().Err(err).Msg("ExecutionEngine: failed to decode command")
			return
		}
		e.execOrderCancelAll(cancelAllCmd)
	}
}

// Start starts the execution engine
func (e *Engine) Start() {
	for _, a := range e.actors {
		a.OnStart()
	}
	log().Info().Msg("ExecutionEngine started")
	e.NotifyReady()
}

// Stop stops the execution engine
func (e *Engine) Stop() {
	for _, a := range e.actors {
		a.OnStop()
	}
	log().Info().Msg("ExecutionEngine stopped")
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect establishes WebSocket connections to all execution clients.
// Subscriptions are already prepared during Init().
func (e *Engine) Connect(ctx context.Context) error {
	return e.router.Connect(ctx)
}

// Disconnect disconnects the execution router
func (e *Engine) Disconnect() {
	e.router.Disconnect()
}

// ============================================================================
// Order Submission
// ============================================================================

func (e *Engine) execOrderSubmit(cmd command.SubmitOrder) {
	e.router.SubmitOrder(cmd.AccountID, cmd.ClientOrderID, cmd.SymbolID, cmd.Side, cmd.OrderType, cmd.TimeInForce, cmd.Price, cmd.Quantity)
}

func (e *Engine) execOrderCancel(cmd command.CancelOrder) {
	order, ok := e.cache.GetOpenOrder(cmd.AccountID, cmd.ClientOrderID)
	if !ok {
		log().Error().Int("accountID", cmd.AccountID).Int("clientOrderID", cmd.ClientOrderID).Msg("Order not found")
		return
	}
	order.OrderStatus = common.OrderStatusCancelling
	e.cache.UpdateOrder(&order)
	log().Info().Int("accountID", cmd.AccountID).Int("clientOrderID", cmd.ClientOrderID).Msg("Order status set to Cancelling (optimistic)")
	e.router.CancelOrder(cmd.AccountID, order.SymbolID, order.OrderID, cmd.ClientOrderID)
}

func (e *Engine) execOrderCancelAll(cmd command.CancelAll) {
	e.router.CancelAllOrders(cmd.AccountID, cmd.SymbolID)
}
