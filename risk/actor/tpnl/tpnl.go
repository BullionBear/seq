package tpnl

import (
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/risk"
	"github.com/mitchellh/mapstructure"
)

func init() {
	risk.Register("tpnl", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewTpnl(cat, bus, c)
	})
}

var _ actor.Actor = (*Tpnl)(nil)

// Tpnl records trades into a shared TpnlState and purges expired entries on
// each execution using FilledAt as the clock source. The corresponding
// TpnlStop checker purges again at risk-check time and computes PnL using
// current mid-prices.
type Tpnl struct {
	actor.ActorBase
	catalog *catalog.Catalog
	cache   *cache.Cache

	accountID int
	cacheKey  string
	state     *cache.TpnlState
}

func NewTpnl(cat *catalog.Catalog, _ *msgbus.MsgBus, c *cache.Cache) *Tpnl {
	return &Tpnl{
		ActorBase: actor.NewActorBase("tpnl", []event.Topic{
			event.TopicEventExecution,
		}),
		catalog: cat,
		cache:   c,
	}
}

type tpnlConfig struct {
	Account  string `yaml:"account"`
	Window   string `yaml:"window"`
	Capacity uint64 `yaml:"capacity"`
}

func (t *Tpnl) OnInit(config map[string]any) {
	var cfg tpnlConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		t.Log().Fatal().Err(err).Msg("Tpnl: failed to create config decoder")
	}
	if err := decoder.Decode(config); err != nil {
		t.Log().Fatal().Err(err).Msg("Tpnl: failed to decode config")
	}

	if cfg.Window == "" {
		t.Log().Fatal().Msg("Tpnl: window is required")
	}
	dur, err := time.ParseDuration(cfg.Window)
	if err != nil {
		t.Log().Fatal().Err(err).Str("window", cfg.Window).Msg("Tpnl: invalid window duration")
	}
	if dur <= 0 {
		t.Log().Fatal().Str("window", cfg.Window).Msg("Tpnl: window must be > 0")
	}
	windowNs := uint64(dur)

	t.accountID = -1
	if cfg.Account != "" {
		acct := t.catalog.GetAccountByName(cfg.Account)
		if acct == nil {
			t.Log().Fatal().Str("account", cfg.Account).Msg("Tpnl: account not found")
		}
		t.accountID = acct.ID
	}

	cap := cfg.Capacity
	if cap == 0 {
		cap = cache.DefaultTpnlCapacity
	}

	t.cacheKey = cache.TpnlCacheKey(t.accountID, windowNs)
	t.state = t.cache.GetOrCreateTpnlState(t.cacheKey, windowNs, cap)

	t.Log().Info().
		Int("account_id", t.accountID).
		Uint64("window_ns", windowNs).
		Uint64("capacity", cap).
		Str("cache_key", t.cacheKey).
		Msg("Tpnl: initialized")
}

func (t *Tpnl) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec, err := event.NewExecutionFromBytes(buf)
		if err != nil {
			return
		}
		t.onExecution(exec)
	}
}

func (t *Tpnl) onExecution(exec event.Execution) {
	if t.accountID != -1 && exec.AccountID != t.accountID {
		return
	}

	t.ensureTokens(exec.SymbolID)

	t.state.AdvanceClock(exec.FilledAt)
	purged := t.state.Purge()

	rec := cache.TpnlTradeRecord{
		SymbolID: exec.SymbolID,
		Side:     exec.Side,
		Qty:      exec.FilledQty,
		Price:    exec.FilledPrice,
		FeeCcyID: exec.FeeCcyID,
		FeeQty:   exec.FeeQty,
		FilledAt: exec.FilledAt,
	}

	if !t.state.AddTrade(rec) {
		t.Log().Warn().
			Uint64("capacity", t.state.Capacity()).
			Msg("Tpnl: ring buffer full, evicted oldest trade")
	}

	baseExp, quoteExp := t.state.GetExposure(exec.SymbolID)
	t.Log().Info().
		Int("symbol_id", exec.SymbolID).
		Int("account_id", exec.AccountID).
		Str("side", exec.Side.String()).
		Float64("filled_qty", exec.FilledQty).
		Float64("filled_price", exec.FilledPrice).
		Float64("fee_qty", exec.FeeQty).
		Int("fee_ccy_id", exec.FeeCcyID).
		Float64("base_exposure", baseExp).
		Float64("quote_change", quoteExp).
		Int("purged", purged).
		Uint64("active_trades", t.state.TradeCount()).
		Msg("Tpnl: execution")
}

func (t *Tpnl) ensureTokens(symbolID int) {
	sym, err := t.catalog.GetSymbol(symbolID)
	if err != nil {
		t.Log().Warn().Int("symbol_id", symbolID).Err(err).Msg("Tpnl: symbol not found, fee attribution may be inaccurate")
		t.state.SetTokens(symbolID, 0, 0)
		return
	}
	t.state.SetTokens(symbolID, sym.BaseToken.ID, sym.QuoteToken.ID)
}
