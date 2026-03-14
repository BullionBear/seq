package tpnl

import (
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
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

// tradeRecord stores a single fill so it can be reverted when it leaves the
// sliding window.
type tradeRecord struct {
	symbolID int
	side     common.Side
	qty      float64
	price    float64
	filledAt uint64 // unix nanoseconds
}

// symbolExposure tracks the net base-currency and quote-currency changes
// produced by trades on a single symbol within the active window.
type symbolExposure struct {
	baseQty     float64 // net base-currency quantity (positive = long)
	quoteChange float64 // cumulative quote-currency change
}

// Tpnl computes a sliding-window trading PnL and writes the result to the
// shared cache so the corresponding TpnlStop risk rule can gate order flow.
type Tpnl struct {
	actor.ActorBase
	catalog *catalog.Catalog
	cache   *cache.Cache

	accountID int    // -1 = all accounts
	windowNs  uint64 // sliding window in nanoseconds
	cacheKey  string // cache.TpnlCacheKey(accountID, windowNs)

	trades   []tradeRecord
	exposure map[int]*symbolExposure
}

// NewTpnl creates an uninitialised Tpnl actor.
func NewTpnl(cat *catalog.Catalog, _ *msgbus.MsgBus, c *cache.Cache) *Tpnl {
	return &Tpnl{
		ActorBase: actor.NewActorBase("tpnl", []event.Topic{
			event.TopicEventExecution,
			event.TopicEventDepthUpdate,
		}),
		catalog:  cat,
		cache:    c,
		exposure: make(map[int]*symbolExposure),
	}
}

type tpnlConfig struct {
	Account string `yaml:"account"` // account name, empty = all
	Window  string `yaml:"window"`  // Go duration, e.g. "5m"
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
	t.windowNs = uint64(dur)

	t.accountID = -1
	if cfg.Account != "" {
		acct := t.catalog.GetAccountByName(cfg.Account)
		if acct == nil {
			t.Log().Fatal().Str("account", cfg.Account).Msg("Tpnl: account not found")
		}
		t.accountID = acct.ID
	}

	t.cacheKey = cache.TpnlCacheKey(t.accountID, t.windowNs)

	t.Log().Info().
		Int("account_id", t.accountID).
		Uint64("window_ns", t.windowNs).
		Str("cache_key", t.cacheKey).
		Msg("Tpnl: initialized")
}

func (t *Tpnl) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		exec := event.NewExecutionFromBytes(buf)
		t.onExecution(exec)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		t.onDepthUpdate(update)
	}
}

func (t *Tpnl) onExecution(exec event.Execution) {
	if t.accountID != -1 && exec.AccountID != t.accountID {
		return
	}

	rec := tradeRecord{
		symbolID: exec.SymbolID,
		side:     exec.Side,
		qty:      exec.FilledQty,
		price:    exec.FilledPrice,
		filledAt: exec.FilledAt,
	}
	t.trades = append(t.trades, rec)
	t.applyTrade(rec, 1)

	t.purgeAndRecalc()
}

func (t *Tpnl) onDepthUpdate(update event.DepthUpdate) {
	if _, ok := t.exposure[update.SymbolID]; !ok {
		return
	}
	t.purgeAndRecalc()
}

// applyTrade adjusts exposure for a single trade. direction=1 to apply,
// direction=-1 to revert.
func (t *Tpnl) applyTrade(rec tradeRecord, direction float64) {
	exp, ok := t.exposure[rec.symbolID]
	if !ok {
		exp = &symbolExposure{}
		t.exposure[rec.symbolID] = exp
	}
	switch rec.side {
	case common.SideBuy:
		exp.baseQty += direction * rec.qty
		exp.quoteChange -= direction * rec.qty * rec.price
	case common.SideSell:
		exp.baseQty -= direction * rec.qty
		exp.quoteChange += direction * rec.qty * rec.price
	}
}

// purgeAndRecalc removes expired trades from the front of the window, reverts
// their effect on exposure, and writes the updated TPNL to the cache.
func (t *Tpnl) purgeAndRecalc() {
	now := uint64(time.Now().UnixNano())
	cutoff := now - t.windowNs

	purged := 0
	for len(t.trades) > 0 && t.trades[0].filledAt < cutoff {
		t.applyTrade(t.trades[0], -1)
		t.trades[0] = tradeRecord{} // avoid retaining pointers
		t.trades = t.trades[1:]
		purged++
	}

	tpnl := 0.0
	for symID, exp := range t.exposure {
		mid, ok := t.cache.GetMidPrice(symID)
		if !ok {
			continue
		}
		tpnl += exp.baseQty*mid + exp.quoteChange
	}

	t.cache.SetTpnl(t.cacheKey, tpnl)

	t.Log().Debug().
		Float64("tpnl", tpnl).
		Int("trades", len(t.trades)).
		Int("purged", purged).
		Msg("Tpnl: updated")
}
