package actor

import (
	"sync"

	coreactor "github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// BalanceEngineHandler is implemented by the portfolio engine.
// It receives event callbacks and lifecycle notifications from BalanceActor.
type BalanceEngineHandler interface {
	OnBalanceUpdate(ev event.BalanceUpdate)
	OnReqBalanceSnapshot(ev event.ReqBalanceSnapshot)
	OnFill(ev event.Fill)
	NotifyReady()
}

// Ensure BalanceActor implements the Actor interface
var _ coreactor.Actor = (*BalanceActor)(nil)

// BalanceActor is an actor owned by the portfolio Engine.
// It subscribes to balance and fill events from the EventBus, routes them to
// the engine's update methods, and notifies the engine when all initial
// balance snapshots have been received.
type BalanceActor struct {
	coreactor.ActorBase
	handler    BalanceEngineHandler
	execRouter *adapter.ExecutionRouter
	accountIDs []int

	// Snapshot tracking
	pending map[int]bool // accountID -> snapshot received
	mu      sync.Mutex
}

// NewBalanceActor creates a new BalanceActor for the given engine handler.
func NewBalanceActor(handler BalanceEngineHandler) *BalanceActor {
	return &BalanceActor{
		ActorBase: coreactor.NewActorBase("portfolio-balance", []event.Topic{
			event.TopicEventBalanceUpdate,
			event.TopicEventReqBalanceSnapshot,
			event.TopicEventFill,
		}),
		handler: handler,
	}
}

// Configure sets the execution router and account IDs.
// Call this from the engine's Init() before registering with the EventBus.
func (b *BalanceActor) Configure(router *adapter.ExecutionRouter, accountIDs []int) {
	b.execRouter = router
	b.accountIDs = accountIDs
}

// OnStart requests initial balance snapshots for all configured accounts.
// Called by the engine's Start() to initiate the ready flow.
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
func (b *BalanceActor) Handle(ev evbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeBalanceUpdate(buf)
		b.handler.OnBalanceUpdate(update)
	case event.TopicEventReqBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqBalanceSnapshot(buf)
		b.handler.OnReqBalanceSnapshot(snapshot)
		b.markSnapshotReceived(snapshot.AccountID)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		b.handler.OnFill(fill)
	}
}

// markSnapshotReceived marks the given account's snapshot as received.
// Calls NotifyReady on the handler once all accounts have been snapshotted.
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

// checkAllDoneLocked returns true if all pending snapshots have been received.
// Must be called with mu held.
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
