package balance

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/portfolio"
	"github.com/mitchellh/mapstructure"
)

func init() {
	portfolio.Register("balance", func(handler portfolio.BalanceEngineHandler) actor.Actor {
		return NewBalanceActor(handler)
	})
}

var _ actor.Actor = (*BalanceActor)(nil)

// BalanceActor is an actor owned by the portfolio Engine.
// It subscribes to balance and fill events from the EventBus, routes them to
// the engine's update methods, and notifies the engine when all initial
// balance snapshots have been received.
type BalanceActor struct {
	actor.ActorBase
	handler portfolio.BalanceEngineHandler
}

// NewBalanceActor creates a new BalanceActor for the given engine handler.
func NewBalanceActor(handler portfolio.BalanceEngineHandler) *BalanceActor {
	return &BalanceActor{
		ActorBase: actor.NewActorBase("portfolio-balance", nil),
		handler:   handler,
	}
}

// OnInit decodes the config for this balance actor and sets subscribed topics.
func (b *BalanceActor) OnInit(config map[string]any) {
	var cfg BalanceConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		b.Log().Error().Err(err).Msg("BalanceActor: failed to create decoder")
		return
	}
	if err := decoder.Decode(config); err != nil {
		b.Log().Error().Err(err).Msg("BalanceActor: failed to decode config")
		return
	}

	if len(cfg.Subscription) > 0 {
		topics, err := event.ParseTopics(cfg.Subscription)
		if err != nil {
			b.Log().Error().Err(err).Msg("BalanceActor: failed to parse subscription topics")
			return
		}
		b.SetTopics(topics)
	}

	b.Log().Info().Strs("subscription", cfg.Subscription).Msg("BalanceActor: initialized from config")
}

// OnStart notifies the engine to request initial balance snapshots.
func (b *BalanceActor) OnStart() {
	b.Log().Info().Msg("BalanceActor: started")
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
	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec := event.NewExecutionFromBytes(buf)
		b.handler.OnExecution(exec)
	}
}
