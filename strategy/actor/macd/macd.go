package macd

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/mitchellh/mapstructure"
)

func init() {
	strategy.Register("macd", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewMACD(cat, bus, c)
	})
}

var _ actor.Actor = (*MACD)(nil)

// MACD is a spot MACD crossover strategy driven by closed kline bars.
// Bullish crossover (histogram crosses above zero) → market buy exec_qty when flat.
// Bearish crossover (histogram crosses below zero) → market sell exec_qty when long.
type MACD struct {
	strategy.StrategyActorBase
	cache *cache.Cache

	symbolID   int
	ticker     string
	accountID  int
	execQty    float64
	fastPeriod int
	slowPeriod int
	sigPeriod  int

	state *macdState

	position float64
	pending  int // in-flight clientOrderID; 0 if idle
	pendSide common.Side
}

// NewMACD creates a new MACD strategy actor.
func NewMACD(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) *MACD {
	return &MACD{
		StrategyActorBase: strategy.NewStrategyActorBase("macd", cat, c, bus, []event.Topic{
			event.TopicEventKline,
			event.TopicEventOrderFilled,
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
			event.TopicEventOrderError,
			event.TopicEventOrderRiskInvalid,
		}),
		cache: c,
	}
}

// OnInit decodes config and resolves symbol / wallet.
func (m *MACD) OnInit(config map[string]any) {
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		m.Log().Panic().Err(err).Msg("macd: failed to create decoder")
		return
	}
	if err = decoder.Decode(config); err != nil {
		m.Log().Panic().Err(err).Msg("macd: failed to decode config")
		return
	}

	if cfg.FastPeriod <= 0 || cfg.SlowPeriod <= 0 || cfg.SignalPeriod <= 0 {
		m.Log().Panic().
			Int("fast_period", cfg.FastPeriod).
			Int("slow_period", cfg.SlowPeriod).
			Int("signal_period", cfg.SignalPeriod).
			Msg("macd: periods must be positive")
		return
	}
	if cfg.FastPeriod >= cfg.SlowPeriod {
		m.Log().Panic().
			Int("fast_period", cfg.FastPeriod).
			Int("slow_period", cfg.SlowPeriod).
			Msg("macd: fast_period must be < slow_period")
		return
	}
	if cfg.ExecQty <= 0 {
		m.Log().Panic().Float64("exec_qty", cfg.ExecQty).Msg("macd: exec_qty must be positive")
		return
	}

	symbol, err := m.GetCatalog().GetSymbolByUniversalTicker(cfg.UniversalTicker)
	if err != nil {
		m.Log().Panic().Err(err).Str("universal_ticker", cfg.UniversalTicker).Msg("macd: symbol not found")
		return
	}
	wallet, err := m.GetCatalog().GetWalletByName(cfg.Wallet)
	if err != nil {
		m.Log().Panic().Err(err).Str("wallet", cfg.Wallet).Msg("macd: wallet not found")
		return
	}

	m.symbolID = symbol.ID
	m.ticker = symbol.UniversalTicker
	m.accountID = wallet.AcctID
	m.execQty = cfg.ExecQty
	m.fastPeriod = cfg.FastPeriod
	m.slowPeriod = cfg.SlowPeriod
	m.sigPeriod = cfg.SignalPeriod
	m.state = newMACDState(cfg.FastPeriod, cfg.SlowPeriod, cfg.SignalPeriod)

	m.Log().Info().
		Str("ticker", m.ticker).
		Int("symbolID", m.symbolID).
		Str("wallet", wallet.Name).
		Int("accountID", m.accountID).
		Int("fast", m.fastPeriod).
		Int("slow", m.slowPeriod).
		Int("signal", m.sigPeriod).
		Float64("execQty", m.execQty).
		Msg("macd: initialized")
}

func (m *MACD) OnStart() {
	m.Log().Info().Str("ticker", m.ticker).Msg("macd: started")
}

func (m *MACD) OnStop() {
	m.Log().Info().Str("ticker", m.ticker).Float64("position", m.position).Msg("macd: stopped")
	if m.pending != 0 {
		m.CancelOrder(m.pending, m.accountID)
		m.pending = 0
	}
}

// Handle dispatches kline and order-lifecycle events.
func (m *MACD) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventKline:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		kline, err := event.NewKlineFromBytes(buf)
		if err != nil {
			m.Log().Error().Err(err).Msg("macd: failed to decode kline")
			return
		}
		m.onKline(kline)
	case event.TopicEventOrderFilled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		filled, err := event.NewOrderFilledFromBytes(buf)
		if err != nil {
			return
		}
		m.onOrderDone(filled.ClientOrderID, true, filled.ExecutedQty)
	case event.TopicEventOrderCanceled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		canceled, err := event.NewOrderCanceledFromBytes(buf)
		if err != nil {
			return
		}
		m.onOrderDone(canceled.ClientOrderID, false, 0)
	case event.TopicEventOrderRejected:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		rejected, err := event.NewOrderRejectedFromBytes(buf)
		if err != nil {
			return
		}
		m.onOrderDone(rejected.ClientOrderID, false, 0)
	case event.TopicEventOrderError:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderErr, err := event.NewOrderErrorFromBytes(buf)
		if err != nil {
			return
		}
		m.onOrderDone(orderErr.ClientOrderID, false, 0)
	case event.TopicEventOrderRiskInvalid:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		invalid, err := event.NewOrderRiskInvalidFromBytes(buf)
		if err != nil {
			return
		}
		m.onOrderDone(invalid.ClientOrderID, false, 0)
	}
}

func (m *MACD) onKline(k event.Kline) {
	if k.SymbolID != m.symbolID || !k.Closed {
		return
	}
	if m.state == nil {
		return
	}

	macdLine, signalLine, hist, ready, xover := m.state.Update(k.Close)
	if !ready {
		return
	}

	m.Log().Debug().
		Float64("close", k.Close).
		Float64("macd", macdLine).
		Float64("signal", signalLine).
		Float64("hist", hist).
		Msg("macd: bar")

	switch xover {
	case crossoverBullish:
		m.tryEnter()
	case crossoverBearish:
		m.tryExit()
	}
}

func (m *MACD) tryEnter() {
	if m.pending != 0 {
		m.Log().Debug().Msg("macd: skip buy, order in flight")
		return
	}
	if m.position > 0 {
		m.Log().Debug().Float64("position", m.position).Msg("macd: skip buy, already long")
		return
	}
	oid := m.SubmitOrder(m.accountID, m.symbolID, common.SideBuy, common.OrderTypeMarket, common.TimeInForceIOC, 0, m.execQty)
	m.pending = oid
	m.pendSide = common.SideBuy
	m.Log().Info().Int("clientOrderID", oid).Float64("qty", m.execQty).Msg("macd: bullish crossover → buy")
}

func (m *MACD) tryExit() {
	if m.pending != 0 {
		m.Log().Debug().Msg("macd: skip sell, order in flight")
		return
	}
	if m.position <= 0 {
		m.Log().Debug().Msg("macd: skip sell, flat")
		return
	}
	qty := m.position
	if qty > m.execQty {
		qty = m.execQty
	}
	oid := m.SubmitOrder(m.accountID, m.symbolID, common.SideSell, common.OrderTypeMarket, common.TimeInForceIOC, 0, qty)
	m.pending = oid
	m.pendSide = common.SideSell
	m.Log().Info().Int("clientOrderID", oid).Float64("qty", qty).Msg("macd: bearish crossover → sell")
}

func (m *MACD) onOrderDone(clientOrderID int, filled bool, executedQty float64) {
	if clientOrderID == 0 || clientOrderID != m.pending {
		return
	}
	if filled {
		qty := executedQty
		if qty <= 0 {
			qty = m.execQty
		}
		switch m.pendSide {
		case common.SideBuy:
			m.position += qty
		case common.SideSell:
			m.position -= qty
			if m.position < 0 {
				m.position = 0
			}
		}
		m.Log().Info().
			Int("clientOrderID", clientOrderID).
			Str("side", m.pendSide.String()).
			Float64("executedQty", qty).
			Float64("position", m.position).
			Msg("macd: order filled")
	} else {
		m.Log().Warn().
			Int("clientOrderID", clientOrderID).
			Str("side", m.pendSide.String()).
			Msg("macd: order failed/canceled")
	}
	m.pending = 0
	m.pendSide = common.SideUnknown
}
