package balance

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/model/common"
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

// Deterministic subscriptions for every balance actor.
var balanceTopics = []event.Topic{
	event.TopicEventRespBalanceSnapshot,
	event.TopicEventBalanceUpdate,
	event.TopicEventExecution,
	event.TopicEventOrderCanceled,
	event.TopicEventOrderNew,
}

var _ actor.Actor = (*BalanceActor)(nil)

// BalanceActor is an actor owned by the portfolio Engine.
// Each instance manages exactly one wallet, filtering incoming events by walletID.
type BalanceActor struct {
	actor.ActorBase
	handler portfolio.BalanceEngineHandler

	wallet     string
	accountID  int
	walletID   int
	walletType common.WalletType
}

// NewBalanceActor creates a new BalanceActor for the given engine handler.
func NewBalanceActor(handler portfolio.BalanceEngineHandler) *BalanceActor {
	return &BalanceActor{
		ActorBase: actor.NewActorBase("portfolio-balance", balanceTopics),
		handler:   handler,
	}
}

// OnInit decodes the wallet config and resolves wallet identity from the catalog.
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

	if cfg.Wallet == "" {
		b.Log().Error().Msg("BalanceActor: wallet name is required in config")
		return
	}

	b.wallet = cfg.Wallet
	accountID, walletID, walletType, err := b.handler.ResolveWallet(cfg.Wallet)
	if err != nil {
		b.Log().Error().Err(err).Str("wallet", cfg.Wallet).Msg("BalanceActor: failed to resolve wallet")
		return
	}
	b.accountID = accountID
	b.walletID = walletID
	b.walletType = walletType

	b.Log().Info().
		Str("wallet", cfg.Wallet).
		Int("accountID", accountID).
		Int("walletID", walletID).
		Str("walletType", walletType.String()).
		Msg("BalanceActor: initialized")
}

// OnStart notifies the engine to request initial balance snapshots.
func (b *BalanceActor) OnStart() {
	b.Log().Info().Str("wallet", b.wallet).Msg("BalanceActor: started")
}

// Handle routes events to the engine, filtering by this actor's wallet.
func (b *BalanceActor) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewBalanceUpdateFromBytes(buf)
		if update.WalletID != b.walletID {
			return
		}
		b.handler.OnBalanceUpdate(update)

	case event.TopicEventRespBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewRespBalanceSnapshotFromBytes(buf)
		if snapshot.WalletID != b.walletID {
			return
		}
		b.handler.OnRespBalanceSnapshot(snapshot)

	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec := event.NewExecutionFromBytes(buf)
		if exec.AccountID != b.accountID {
			return
		}
		b.handler.OnExecution(exec)
	}
}
