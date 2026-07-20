package balance

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/ledger"
	"github.com/mitchellh/mapstructure"
)

// EngineHandler defines what the balance actor needs from the ledger engine.
type EngineHandler interface {
	ResolveWallet(name string) (accountID int, walletID int, walletType common.WalletType, err error)
	NotifyReady()
}

func init() {
	ledger.Register("balance", func(handler any) actor.Actor {
		return NewBalanceActor(handler.(EngineHandler))
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

// BalanceActor is an actor owned by the ledger Engine.
// Each instance manages exactly one wallet, filtering incoming events by walletID.
// It writes balance state directly to cache for other engines/strategies to read.
type BalanceActor struct {
	actor.ActorBase
	handler EngineHandler
	cache   *cache.Cache
	catalog *catalog.Catalog

	wallet           string
	accountID        int
	walletID         int
	walletType       common.WalletType
	snapshotReceived bool
}

// NewBalanceActor creates a new BalanceActor for the given engine handler.
func NewBalanceActor(handler EngineHandler) *BalanceActor {
	return &BalanceActor{
		ActorBase: actor.NewActorBase("ledger-balance", balanceTopics),
		handler:   handler,
	}
}

// SetCache injects the shared cache for balance writes.
func (b *BalanceActor) SetCache(c *cache.Cache) { b.cache = c }

// SetCatalog injects the catalog for token name resolution.
func (b *BalanceActor) SetCatalog(cat *catalog.Catalog) { b.catalog = cat }

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

func (b *BalanceActor) OnStart() {
	b.Log().Info().Str("wallet", b.wallet).Msg("BalanceActor: started")
}

// Handle processes events, filtering by this actor's wallet, and writes to cache.
func (b *BalanceActor) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventBalanceUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update, err := event.NewBalanceUpdateFromBytes(buf)
		if err != nil {
			b.Log().Error().Err(err).Msg("BalanceActor: failed to decode event")
			return
		}
		if update.WalletID != b.walletID {
			return
		}
		b.onBalanceUpdate(update)

	case event.TopicEventRespBalanceSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot, err := event.NewRespBalanceSnapshotFromBytes(buf)
		if err != nil {
			b.Log().Error().Err(err).Msg("BalanceActor: failed to decode event")
			return
		}
		if snapshot.WalletID != b.walletID {
			return
		}
		b.onRespBalanceSnapshot(snapshot)

	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec, err := event.NewExecutionFromBytes(buf)
		if err != nil {
			b.Log().Error().Err(err).Msg("BalanceActor: failed to decode event")
			return
		}
		if exec.AccountID != b.accountID {
			return
		}
		b.onExecution(exec)
	}
}

func (b *BalanceActor) onBalanceUpdate(ev event.BalanceUpdate) {
	for _, bal := range ev.Balances {
		b.cache.SetBalance(b.accountID, bal.TokenID, bal.Available, bal.Locked, bal.Total)
	}
	b.Log().Debug().
		Int("accountID", ev.AccountID).
		Str("walletType", b.walletType.String()).
		Int("balanceCount", len(ev.Balances)).
		Msg("BalanceActor: balance updated")
}

func (b *BalanceActor) onRespBalanceSnapshot(ev event.RespBalanceSnapshot) {
	b.cache.SetAccountBalances(b.accountID, ev.Balances)

	b.Log().Info().
		Int("accountID", ev.AccountID).
		Str("walletType", b.walletType.String()).
		Int("balanceCount", len(ev.Balances)).
		Msg("BalanceActor: balance snapshot received")

	for _, bal := range ev.Balances {
		if bal.Total > 0 {
			b.Log().Debug().
				Int("accountID", ev.AccountID).
				Str("walletType", b.walletType.String()).
				Str("token", b.tokenName(bal.TokenID)).
				Float64("available", bal.Available).
				Float64("locked", bal.Locked).
				Float64("total", bal.Total).
				Msg("BalanceActor: token balance initialized")
		}
	}

	if !b.snapshotReceived {
		b.snapshotReceived = true
		b.handler.NotifyReady()
	}
}

func (b *BalanceActor) onExecution(ev event.Execution) {
	b.Log().Debug().
		Int("clientOrderID", ev.ClientOrderID).
		Int("accountID", ev.AccountID).
		Int("symbolID", ev.SymbolID).
		Int("fillID", ev.FillID).
		Float64("qty", ev.FilledQty).
		Float64("price", ev.FilledPrice).
		Float64("fee", ev.FeeQty).
		Msg("BalanceActor: fill received")
}

func (b *BalanceActor) tokenName(tokenID int) string {
	if b.catalog == nil {
		return "unknown"
	}
	tok, err := b.catalog.GetToken(tokenID)
	if err != nil {
		return "unknown"
	}
	return tok.Name
}
