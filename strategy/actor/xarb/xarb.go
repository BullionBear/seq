package xarb

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
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

	// Count
	quotingCount int
	hedgingCount int
}

// NewXArb creates a new XArb strategy.
func NewXArb() *XArb {
	return &XArb{
		StrategyBase:  strategy.NewStrategyBase("xarb"),
		quotingSymbol: cpanel.Symbol{},
		hedgingSymbol: cpanel.Symbol{},

		quotingAccount: cpanel.Account{},
		hedgingAccount: cpanel.Account{},

		quotingCount: 0,
		hedgingCount: 0,
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
func (x *XArb) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewDepthSnapshotFromBytes(buf)
		x.OnDepthSnapshot(snapshot)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		x.OnDepthUpdate(update)
	case event.TopicEventTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := event.NewTickFromBytes(buf)
		x.OnTick(tick)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := event.NewFillFromBytes(buf)
		x.OnFill(fill)
	case event.TopicEventRespDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewRespDepthSnapshotFromBytes(buf)
		x.OnRespDepthSnapshot(snapshot)
	}
}

// OnDepthUpdate processes depth updates.
// Note: Snapshot requests are now handled automatically by DataEngine.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID

	bookReady := x.IsSymbolReady(symbolID)
	if !bookReady {
		log().Warn().Int("symbolID", symbolID).Msg("Orderbook not ready")
		return
	}
	switch symbolID {
	case x.quotingSymbol.ID:
		x.quotingCount++
	case x.hedgingSymbol.ID:
		x.hedgingCount++
	}
	log().Info().Int("quotingCount", x.quotingCount).Int("hedgingCount", x.hedgingCount).Msg("Depth update received")

	if symbolID == x.quotingSymbol.ID && x.quotingCount%10 == 0 {
		x.printTop5(x.quotingSymbol.ID)
	}
	if symbolID == x.hedgingSymbol.ID && x.hedgingCount%10 == 0 {
		x.printTop5(x.hedgingSymbol.ID)
	}

}

// OnRespDepthSnapshot processes the response to a depth snapshot request.
func (x *XArb) OnRespDepthSnapshot(snapshot event.RespDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log().Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asks", len(snapshot.Asks)).
		Int("bids", len(snapshot.Bids)).
		Msg("RespDepthSnapshot received")
}

// OnFill processes fill events.
func (x *XArb) OnFill(fill event.Fill) {}

// ============================================================================
// Portfolio Access Methods
// ============================================================================

// printTop5 prints the top 5 bid and ask levels for a symbol.
func (x *XArb) printTop5(symbolID int) {
	bids, asks := x.GetDepth(symbolID, 5)
	log().Info().Int("symbolID", symbolID).Int("bids", len(bids)).Int("asks", len(asks)).Msg("Top 5 levels")
	for i := 0; i < len(bids); i++ {
		log().Info().Int("symbolID", symbolID).Int("bidIndex", i).Float64("bidPrice", bids[i].Price).Float64("bidQty", bids[i].Quantity).Msg("Bid")
	}
	for i := 0; i < len(asks); i++ {
		log().Info().Int("symbolID", symbolID).Int("askIndex", i).Float64("askPrice", asks[i].Price).Float64("askQty", asks[i].Quantity).Msg("Ask")
	}
}

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
