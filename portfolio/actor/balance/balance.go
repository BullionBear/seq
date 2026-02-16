package balance

import (
	"sync"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/portfolio"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	portfolio.Register("balance", func(handler portfolio.BalanceEngineHandler) actor.Actor {
		return NewBalanceActor(handler)
	})
}

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure BalanceActor implements the Actor interface
var _ actor.Actor = (*BalanceActor)(nil)

// BalanceActor is an actor owned by the portfolio Engine.
// It subscribes to balance and fill events from the EventBus, routes them to
// the engine's update methods, and notifies the engine when all initial
// balance snapshots have been received.
type BalanceActor struct {
	actor.ActorBase
	handler    portfolio.BalanceEngineHandler
	execRouter *adapter.ExecutionRouter
	accountIDs []int

	// Config-derived fields
	accountID int
	account   string

	// Snapshot tracking
	pending map[int]bool // accountID -> snapshot received
	mu      sync.Mutex
}

// NewBalanceActor creates a new BalanceActor for the given engine handler.
func NewBalanceActor(handler portfolio.BalanceEngineHandler) *BalanceActor {
	return &BalanceActor{
		ActorBase: actor.NewActorBase("portfolio-balance", []event.Topic{
			event.TopicEventBalanceUpdate,
			event.TopicEventRespBalanceSnapshot,
			event.TopicEventFill,
		}),
		handler: handler,
	}
}

// Configure sets the execution router and account IDs.
// Implements portfolio.BalanceConfigurer.
func (b *BalanceActor) Configure(router *adapter.ExecutionRouter, accountIDs []int) {
	b.execRouter = router
	b.accountIDs = accountIDs
}

// OnInit decodes the config for this balance actor.
func (b *BalanceActor) OnInit(config map[string]any) {
	var cfg BalanceConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		log().Error().Err(err).Msg("BalanceActor: failed to create decoder")
		return
	}
	if err := decoder.Decode(config); err != nil {
		log().Error().Err(err).Msg("BalanceActor: failed to decode config")
		return
	}

	b.accountID = cfg.ID
	b.account = cfg.Account
	log().Info().Int("accountID", b.accountID).Str("account", b.account).Msg("BalanceActor: initialized from config")
}

// OnStart requests initial balance snapshots for all configured accounts.
func (b *BalanceActor) OnStart() {
	if b.execRouter == nil || len(b.accountIDs) == 0 {
		log().Info().Msg("BalanceActor: no router or accounts configured, notifying ready immediately")
		b.handler.NotifyReady()
		return
	}

	b.mu.Lock()
	b.pending = make(map[int]bool)
	for _, id := range b.accountIDs {
		b.pending[id] = false
	}
	b.mu.Unlock()

	for _, id := range b.accountIDs {
		if err := b.execRouter.ReqBalanceSnapshot(id); err != nil {
			log().Error().Err(err).Int("accountID", id).Msg("BalanceActor: failed to request balance snapshot")
		} else {
			log().Debug().Int("accountID", id).Msg("BalanceActor: requested balance snapshot")
		}
	}

	log().Info().Msg("BalanceActor: started, waiting for balance snapshots")
}

// Handle routes events to the engine's update methods.
func (b *BalanceActor) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewBalanceUpdateFromBytes(buf)
		b.handler.OnBalanceUpdate(update)
	case event.TopicEventRespBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewRespBalanceSnapshotFromBytes(buf)
		b.handler.OnRespBalanceSnapshot(snapshot)
		b.markSnapshotReceived(snapshot.AccountID)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := event.NewFillFromBytes(buf)
		b.handler.OnFill(fill)
	}
}

func (b *BalanceActor) markSnapshotReceived(accountID int) {
	b.mu.Lock()
	if b.pending == nil {
		b.mu.Unlock()
		return
	}
	b.pending[accountID] = true
	allDone := b.checkAllDoneLocked()
	b.mu.Unlock()

	if allDone {
		log().Info().Msg("BalanceActor: all balance snapshots received, notifying engine ready")
		b.handler.NotifyReady()
	}
}

func (b *BalanceActor) checkAllDoneLocked() bool {
	if len(b.pending) == 0 {
		return false
	}
	for _, received := range b.pending {
		if !received {
			return false
		}
	}
	return true
}
