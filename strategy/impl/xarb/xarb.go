package xarb

import (
	"fmt"
	"strings"
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Ensure XArb implements the Actor interface
var _ actor.Actor = (*XArb)(nil)

// XArb is a cross-exchange arbitrage strategy.
type XArb struct {
	*strategy.StrategyBase // Embed StrategyBase for Actor + StrategyCommon
	quotingSymbol          cpanel.Symbol
	hedgingSymbol          cpanel.Symbol

	// Account IDs for trading
	quotingAccount cpanel.Account
	hedgingAccount cpanel.Account
}

// NewXArb creates a new XArb strategy.
func NewXArb() *XArb {
	return &XArb{
		StrategyBase:  strategy.NewStrategyBase("xarb"),
		quotingSymbol: cpanel.Symbol{},
		hedgingSymbol: cpanel.Symbol{},

		quotingAccount: cpanel.Account{},
		hedgingAccount: cpanel.Account{},
	}
}

// OnInit initializes the strategy with configuration.
func (x *XArb) OnInit() {
	// Get strategy-specific config from StrategyBase
	strategyConfig := x.StrategyConfig()
	if strategyConfig == nil {
		log().Error().Msg("strategy config is nil")
		return
	}

	// Convert map[string]any to XArbConfig struct via YAML re-marshaling
	var xarbConfig XArbConfig
	yamlData, err := yaml.Marshal(strategyConfig)
	if err != nil {
		log().Error().Err(err).Msg("failed to marshal strategy config")
		return
	}
	if err := yaml.Unmarshal(yamlData, &xarbConfig); err != nil {
		log().Error().Err(err).Msg("failed to unmarshal strategy config")
		return
	}

	// Resolve trading symbols
	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.QuotingSymbolUniversalTicker)
	if err != nil {
		log().Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.HedgingSymbolUniversalTicker)
	if err != nil {
		log().Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol
	log().Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.UniversalTicker, quotingSymbol.ID)
	log().Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.UniversalTicker, hedgingSymbol.ID)

	// Resolve trading accounts
	if xarbConfig.QuotingAccount != "" {
		quotingAccount := x.GetCatalog().GetAccountByName(xarbConfig.QuotingAccount)
		if quotingAccount != nil {
			x.quotingAccount = *quotingAccount
			log().Info().Msgf("Quoting account: %s(%d)", quotingAccount.Name, quotingAccount.ID)
		} else {
			log().Warn().Str("account", xarbConfig.QuotingAccount).Msg("quoting account not found")
		}
	}

	if xarbConfig.HedgingAccount != "" {
		hedgingAccount := x.GetCatalog().GetAccountByName(xarbConfig.HedgingAccount)
		if hedgingAccount != nil {
			x.hedgingAccount = *hedgingAccount
			log().Info().Msgf("Hedging account: %s(%d)", hedgingAccount.Name, hedgingAccount.ID)
		} else {
			log().Warn().Str("account", xarbConfig.HedgingAccount).Msg("hedging account not found")
		}
	}
}

// OnStart is called when the strategy starts.
// Note: Data subscriptions are now handled by the config - no manual Subscribe/Connect needed.
func (x *XArb) OnStart() {
	log().Info().Msgf("XArb strategy started for symbols: %s, %s",
		x.quotingSymbol.UniversalTicker, x.hedgingSymbol.UniversalTicker)
}

// OnStop is called when the strategy stops.
func (x *XArb) OnStop() {
	log().Info().Msg("XArb strategy stopped")
}

// Handle overrides StrategyBase.Handle to dispatch events to XArb's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (x *XArb) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		x.OnDepthSnapshot(snapshot)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		x.OnDepthUpdate(update)
	case event.TopicEventTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := evbus.DeserializeTick(buf)
		x.OnTick(tick)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		x.OnFill(fill)
	case event.TopicEventReqDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqDepthSnapshot(buf)
		x.OnReqDepthSnapshot(snapshot)
	}
}

// OnDepthSnapshot processes depth snapshots.
func (x *XArb) OnDepthSnapshot(snapshot event.DepthSnapshot) {
	log().Info().Msgf("Depth snapshot: %d %d %d %d", snapshot.SymbolID, snapshot.DepthID, len(snapshot.Bids), len(snapshot.Asks))
}

// OnDepthUpdate processes depth updates.
// Note: Snapshot requests are now handled automatically by DataEngine.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID

	// Check orderbook state
	bookState, exists := x.GetBookState(symbolID)
	if !exists {
		log().Warn().Int("symbolID", symbolID).Msg("Orderbook not registered")
		return
	}

	// If ready, print top 5 levels
	if x.IsSymbolReady(symbolID) {
		x.printTop5(symbolID)
	} else {
		log().Debug().
			Int("symbolID", symbolID).
			Str("state", bookState.String()).
			Msg("Depth update received, orderbook not ready")
	}
}

// OnReqDepthSnapshot processes the response to a depth snapshot request.
func (x *XArb) OnReqDepthSnapshot(snapshot event.ReqDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log().Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asks", len(snapshot.Asks)).
		Int("bids", len(snapshot.Bids)).
		Msg("ReqDepthSnapshot received")
}

// printTop5 prints the top 5 bid and ask levels for a symbol.
func (x *XArb) printTop5(symbolID int) {
	bids, asks := x.GetDepth(symbolID, 5)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n=== Orderbook [Symbol %d] ===\n", symbolID))
	sb.WriteString(fmt.Sprintf("%-12s | %-15s || %-15s | %-12s\n", "Bid Qty", "Bid Price", "Ask Price", "Ask Qty"))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	maxLen := len(bids)
	if len(asks) > maxLen {
		maxLen = len(asks)
	}

	for i := 0; i < maxLen; i++ {
		var bidQty, bidPrice, askPrice, askQty string

		if i < len(bids) {
			bidQty = fmt.Sprintf("%.4f", bids[i].Quantity)
			bidPrice = fmt.Sprintf("%.4f", bids[i].Price)
		} else {
			bidQty = ""
			bidPrice = ""
		}

		if i < len(asks) {
			askPrice = fmt.Sprintf("%.4f", asks[i].Price)
			askQty = fmt.Sprintf("%.4f", asks[i].Quantity)
		} else {
			askPrice = ""
			askQty = ""
		}

		sb.WriteString(fmt.Sprintf("%-12s | %-15s || %-15s | %-12s\n", bidQty, bidPrice, askPrice, askQty))
	}

	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339Nano)))
	// log().Info().Msg(sb.String())
}

// OnTick processes tick events.
func (x *XArb) OnTick(tick event.Tick) {}

// OnFill processes fill events.
func (x *XArb) OnFill(fill event.Fill) {}

// ============================================================================
// Portfolio Access Methods
// ============================================================================

// GetQuotingAccountID returns the account ID for quoting
func (x *XArb) GetQuotingAccountID() int {
	return x.quotingAccount.ID
}

// GetHedgingAccountID returns the account ID for hedging
func (x *XArb) GetHedgingAccountID() int {
	return x.hedgingAccount.ID
}

// GetQuotingBalance returns the available balance for a token in the quoting account
func (x *XArb) GetQuotingBalance(tokenID int) float64 {
	if x.quotingAccount.ID == 0 {
		return 0
	}
	return x.GetAvailable(x.quotingAccount.ID, tokenID)
}

// GetHedgingBalance returns the available balance for a token in the hedging account
func (x *XArb) GetHedgingBalance(tokenID int) float64 {
	if x.hedgingAccount.ID == 0 {
		return 0
	}
	return x.GetAvailable(x.hedgingAccount.ID, tokenID)
}
