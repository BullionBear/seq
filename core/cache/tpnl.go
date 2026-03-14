package cache

import (
	"fmt"

	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/common"
)

const DefaultTpnlCapacity uint64 = 4096

// TpnlCacheKey builds the cache key that links a TPNL actor to its checker.
func TpnlCacheKey(accountID int, windowNs uint64) string {
	return fmt.Sprintf("tpnl:%d:%d", accountID, windowNs)
}

// TpnlTradeRecord stores a single fill in the sliding window.
type TpnlTradeRecord struct {
	SymbolID int
	Side     common.Side
	Qty      float64
	Price    float64
	FeeCcyID int
	FeeQty   float64
	FilledAt uint64
}

// TpnlTokenPair caches the base/quote token IDs for a symbol.
type TpnlTokenPair struct {
	BaseTokenID  int
	QuoteTokenID int
}

// TpnlSymbolExposure holds the running base and quote exposure for a single
// symbol within a TPNL window.
type TpnlSymbolExposure struct {
	SymbolID    int
	BaseQty     float64
	QuoteChange float64
}

type tpnlExposure struct {
	baseQty     float64
	quoteChange float64
}

// TpnlState holds the shared mutable state for a TPNL (account, window) pair.
// The TPNL actor adds trades and purges on executions; the checker purges on
// risk checks and reads exposures. All access is from the single dispatch thread.
type TpnlState struct {
	trades   *mem.SPSCRingBuffer[TpnlTradeRecord]
	exposure map[int]*tpnlExposure
	tokens   map[int]*TpnlTokenPair
	nowNs    uint64
	windowNs uint64
}

// NewTpnlState creates a new TpnlState with the given window and ring buffer capacity.
func NewTpnlState(windowNs, capacity uint64) *TpnlState {
	return &TpnlState{
		trades:   mem.NewSPSCRingBuffer[TpnlTradeRecord](capacity),
		exposure: make(map[int]*tpnlExposure),
		tokens:   make(map[int]*TpnlTokenPair),
		windowNs: windowNs,
	}
}

// SetTokens registers the base/quote token IDs for a symbol (idempotent).
func (s *TpnlState) SetTokens(symbolID, baseTokenID, quoteTokenID int) {
	if _, ok := s.tokens[symbolID]; ok {
		return
	}
	s.tokens[symbolID] = &TpnlTokenPair{
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
	}
}

// AdvanceClock moves the internal clock forward to ts if ts > current.
func (s *TpnlState) AdvanceClock(ts uint64) {
	if ts > s.nowNs {
		s.nowNs = ts
	}
}

// AddTrade appends a trade to the ring buffer and applies it to exposure.
// Returns true on success. If the buffer is full, the oldest trade is
// force-evicted and the method returns false.
func (s *TpnlState) AddTrade(rec TpnlTradeRecord) bool {
	ok := true
	if !s.trades.Write(rec) {
		oldest, _ := s.trades.Read()
		s.revertTrade(oldest)
		s.trades.Write(rec)
		ok = false
	}
	s.applyTrade(rec)
	return ok
}

// Purge removes expired trades from the buffer head and reverts their
// effect on exposure. Returns the number of trades purged.
func (s *TpnlState) Purge() int {
	if s.nowNs <= s.windowNs {
		return 0
	}
	cutoff := s.nowNs - s.windowNs

	purged := 0
	for {
		rec, ok := s.trades.Peek()
		if !ok || rec.FilledAt >= cutoff {
			break
		}
		s.trades.Read()
		s.revertTrade(rec)
		purged++
	}
	return purged
}

// Exposures returns a snapshot of all non-zero per-symbol exposures.
func (s *TpnlState) Exposures() []TpnlSymbolExposure {
	result := make([]TpnlSymbolExposure, 0, len(s.exposure))
	for symID, exp := range s.exposure {
		if exp.baseQty == 0 && exp.quoteChange == 0 {
			continue
		}
		result = append(result, TpnlSymbolExposure{
			SymbolID:    symID,
			BaseQty:     exp.baseQty,
			QuoteChange: exp.quoteChange,
		})
	}
	return result
}

// GetExposure returns the exposure for a specific symbol.
func (s *TpnlState) GetExposure(symbolID int) (baseQty, quoteChange float64) {
	exp := s.exposure[symbolID]
	if exp == nil {
		return 0, 0
	}
	return exp.baseQty, exp.quoteChange
}

func (s *TpnlState) NowNs() uint64     { return s.nowNs }
func (s *TpnlState) TradeCount() uint64 { return s.trades.Count() }
func (s *TpnlState) Capacity() uint64   { return s.trades.Capacity() }

func (s *TpnlState) applyTrade(rec TpnlTradeRecord) {
	exp, ok := s.exposure[rec.SymbolID]
	if !ok {
		exp = &tpnlExposure{}
		s.exposure[rec.SymbolID] = exp
	}
	switch rec.Side {
	case common.SideBuy:
		exp.baseQty += rec.Qty
		exp.quoteChange -= rec.Qty * rec.Price
	case common.SideSell:
		exp.baseQty -= rec.Qty
		exp.quoteChange += rec.Qty * rec.Price
	}

	if rec.FeeQty == 0 {
		return
	}
	tok := s.tokens[rec.SymbolID]
	if tok == nil {
		return
	}
	switch rec.FeeCcyID {
	case tok.BaseTokenID:
		exp.baseQty -= rec.FeeQty
	case tok.QuoteTokenID:
		exp.quoteChange -= rec.FeeQty
	default:
		exp.quoteChange -= rec.FeeQty * rec.Price
	}
}

func (s *TpnlState) revertTrade(rec TpnlTradeRecord) {
	exp := s.exposure[rec.SymbolID]
	if exp == nil {
		return
	}
	switch rec.Side {
	case common.SideBuy:
		exp.baseQty -= rec.Qty
		exp.quoteChange += rec.Qty * rec.Price
	case common.SideSell:
		exp.baseQty += rec.Qty
		exp.quoteChange -= rec.Qty * rec.Price
	}

	if rec.FeeQty == 0 {
		return
	}
	tok := s.tokens[rec.SymbolID]
	if tok == nil {
		return
	}
	switch rec.FeeCcyID {
	case tok.BaseTokenID:
		exp.baseQty += rec.FeeQty
	case tok.QuoteTokenID:
		exp.quoteChange += rec.FeeQty
	default:
		exp.quoteChange += rec.FeeQty * rec.Price
	}
}
