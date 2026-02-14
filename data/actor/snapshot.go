package actor

import (
	"sync"

	coreactor "github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/data/ob"
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure Coordinator implements the Actor interface
var _ coreactor.Actor = (*Coordinator)(nil)

// Coordinator is an actor owned by DataEngine.
// It watches depth updates and auto-requests HTTP snapshots when an
// orderbook is in StateWaitForSnapshot, ensuring the book can sync.
// It clears the in-flight flag when the snapshot arrives so future
// resets (gap detection) can trigger a fresh request.
type Coordinator struct {
	coreactor.ActorBase
	ob     *ob.OrderBook
	router *adapter.DataRouter

	mu      sync.Mutex
	pending map[int]bool // symbolID -> snapshot request in-flight
}

// NewCoordinator creates a new snapshot Coordinator actor.
func NewCoordinator(ob *ob.OrderBook, router *adapter.DataRouter) *Coordinator {
	return &Coordinator{
		ActorBase: coreactor.NewActorBase("snapshot-coordinator", []event.Topic{
			event.TopicEventDepthUpdate,
			event.TopicEventDepthSnapshot,
		}),
		ob:      ob,
		router:  router,
		pending: make(map[int]bool),
	}
}

// Handle routes events to the appropriate handler.
func (c *Coordinator) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		c.onDepthUpdate(update)
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		c.onDepthSnapshot(snapshot)
	}
}

// onDepthUpdate checks if the orderbook needs a snapshot and requests one if not already pending.
func (c *Coordinator) onDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID

	state, exists := c.ob.GetBookState(symbolID)
	if !exists {
		return
	}

	if state != ob.StateWaitForSnapshot {
		return
	}

	c.mu.Lock()
	if c.pending[symbolID] {
		c.mu.Unlock()
		return
	}
	c.pending[symbolID] = true
	c.mu.Unlock()

	log().Debug().Int("symbolID", symbolID).Msg("SnapshotCoordinator: requesting depth snapshot")
	if err := c.router.ReqDepthSnapshot(symbolID); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("SnapshotCoordinator: failed to request depth snapshot")
		c.mu.Lock()
		c.pending[symbolID] = false
		c.mu.Unlock()
	}
}

// onDepthSnapshot clears the pending flag so a future reset can trigger a new request.
func (c *Coordinator) onDepthSnapshot(snapshot event.DepthSnapshot) {
	c.mu.Lock()
	delete(c.pending, snapshot.SymbolID)
	c.mu.Unlock()
	log().Debug().Int("symbolID", snapshot.SymbolID).Msg("SnapshotCoordinator: snapshot received, cleared pending flag")
}
