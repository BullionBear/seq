package sma

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
	strategy.Register("sma", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewSMA(cat, bus, c)
	})
}

var _ actor.Actor = (*SMA)(nil)

// SMA computes a simple moving average over the last n closed kline closes.
// On the first live kline it requests the previous n-1 bars, seeds a circular
// buffer, then overwrites the oldest close on each subsequent closed bar.
type SMA struct {
	strategy.StrategyActorBase

	symbolID int
	ticker   string
	interval common.Interval
	period   int

	ring *closeRing

	histRequested bool
	histReady     bool
	firstStart    uint64
	pending       []float64 // closed closes while waiting for hist
}

// NewSMA creates a new SMA strategy actor.
func NewSMA(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) *SMA {
	return &SMA{
		StrategyActorBase: strategy.NewStrategyActorBase("sma", cat, c, bus, []event.Topic{
			event.TopicEventKline,
			event.TopicEventRespHistoricalKline,
		}),
	}
}

// OnInit decodes config and resolves the symbol / interval.
func (s *SMA) OnInit(config map[string]any) {
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		s.Log().Panic().Err(err).Msg("sma: failed to create decoder")
		return
	}
	if err = decoder.Decode(config); err != nil {
		s.Log().Panic().Err(err).Msg("sma: failed to decode config")
		return
	}

	if cfg.Period <= 0 {
		s.Log().Panic().Int("period", cfg.Period).Msg("sma: period must be positive")
		return
	}
	interval, err := common.ParseInterval(cfg.Interval)
	if err != nil {
		s.Log().Panic().Err(err).Str("interval", cfg.Interval).Msg("sma: invalid interval")
		return
	}
	symbol, err := s.GetCatalog().GetSymbolByUniversalTicker(cfg.UniversalTicker)
	if err != nil {
		s.Log().Panic().Err(err).Str("universal_ticker", cfg.UniversalTicker).Msg("sma: symbol not found")
		return
	}

	s.symbolID = symbol.ID
	s.ticker = symbol.UniversalTicker
	s.interval = interval
	s.period = cfg.Period
	s.ring = newCloseRing(cfg.Period)
	if cfg.Period == 1 {
		s.histReady = true
	}

	s.Log().Info().
		Str("ticker", s.ticker).
		Int("symbolID", s.symbolID).
		Str("interval", s.interval.String()).
		Int("period", s.period).
		Msg("sma: initialized")
}

func (s *SMA) OnStart() {
	s.Log().Info().Str("ticker", s.ticker).Msg("sma: started")
}

func (s *SMA) OnStop() {
	s.Log().Info().
		Str("ticker", s.ticker).
		Int("filled", s.ring.filled()).
		Msg("sma: stopped")
}

// Handle dispatches live kline and historical kline responses.
func (s *SMA) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventKline:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		kline, err := event.NewKlineFromBytes(buf)
		if err != nil {
			s.Log().Error().Err(err).Msg("sma: failed to decode kline")
			return
		}
		s.onKline(kline)
	case event.TopicEventRespHistoricalKline:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		resp, err := event.NewRespHistoricalKlineFromBytes(buf)
		if err != nil {
			s.Log().Error().Err(err).Msg("sma: failed to decode historical kline")
			return
		}
		s.onHist(resp)
	}
}

func (s *SMA) onKline(k event.Kline) {
	if k.SymbolID != s.symbolID || k.Interval != s.interval {
		return
	}

	if !s.histRequested && s.period > 1 {
		s.histRequested = true
		s.firstStart = k.StartTime
		end := k.StartTime
		if end > 0 {
			end-- // exclusive of the live bar's start
		}
		s.Log().Info().
			Int("limit", s.period-1).
			Uint64("endTimeNs", end).
			Msg("sma: requesting previous klines")
		s.ReqHistoricalKline(s.symbolID, s.interval, 0, end, s.period-1)
	}

	if !k.Closed {
		return
	}
	if !s.histReady {
		s.pending = append(s.pending, k.Close)
		return
	}
	s.pushAndLog(k.Close, k.StartTime)
}

func (s *SMA) onHist(resp event.RespHistoricalKline) {
	if resp.SymbolID != s.symbolID || resp.Interval != s.interval {
		return
	}
	if s.histReady {
		return
	}

	applied := 0
	for _, bar := range resp.Bars {
		if s.firstStart > 0 && bar.StartTime >= s.firstStart {
			continue
		}
		s.ring.push(bar.Close)
		applied++
	}
	s.histReady = true

	for _, c := range s.pending {
		s.pushAndLog(c, 0)
	}
	s.pending = nil

	s.Log().Info().
		Int("histBars", applied).
		Int("filled", s.ring.filled()).
		Int("period", s.period).
		Msg("sma: historical warmup applied")

	if sma, ready := s.currentSMA(); ready {
		s.Log().Info().
			Float64("sma", sma).
			Int("period", s.period).
			Msg("sma: ready")
	}
}

func (s *SMA) pushAndLog(close float64, startTime uint64) {
	sma, ready := s.ring.push(close)
	if !ready {
		s.Log().Debug().
			Float64("close", close).
			Int("filled", s.ring.filled()).
			Int("period", s.period).
			Msg("sma: warming")
		return
	}
	ev := s.Log().Info().
		Float64("close", close).
		Float64("sma", sma).
		Int("period", s.period)
	if startTime > 0 {
		ev = ev.Uint64("startTime", startTime)
	}
	ev.Msg("sma: updated")
}

func (s *SMA) currentSMA() (float64, bool) {
	if s.ring.filled() < s.ring.period() {
		return 0, false
	}
	return s.ring.sum / float64(s.ring.period()), true
}
