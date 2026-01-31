package xarb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/BullionBear/seq/strategy/actor"
	"github.com/BullionBear/seq/strategy/actor/ob"
	"gopkg.in/yaml.v3"
)

var log = logger.Get()

// Ensure XArb implements the Actor interface
var _ actor.Actor = (*XArb)(nil)

// XArb is a cross-exchange arbitrage strategy.
type XArb struct {
	*strategy.StrategyBase // Embed StrategyBase for Actor + StrategyCommon
	quotingSymbol          cpanel.Symbol
	hedgingSymbol          cpanel.Symbol

	// IsBookInFlight tracks whether a snapshot has been requested but not yet received
	IsBookInFlight map[int]bool // symbolID -> is snapshot requested but not arrived
}

// NewXArb creates a new XArb strategy.
func NewXArb() *XArb {
	return &XArb{
		StrategyBase:   strategy.NewStrategyBase("xarb"),
		quotingSymbol:  cpanel.Symbol{},
		hedgingSymbol:  cpanel.Symbol{},
		IsBookInFlight: make(map[int]bool),
	}
}

// OnInit initializes the strategy with configuration.
func (x *XArb) OnInit() {
	// Initialize IsBookInFlight map
	x.IsBookInFlight = make(map[int]bool)

	// Get strategy-specific config from StrategyBase
	strategyConfig := x.StrategyConfig()
	if strategyConfig == nil {
		log.Error().Msg("strategy config is nil")
		return
	}

	// Convert map[string]any to XArbConfig struct via YAML re-marshaling
	var xarbConfig XArbConfig
	yamlData, err := yaml.Marshal(strategyConfig)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal strategy config")
		return
	}
	if err := yaml.Unmarshal(yamlData, &xarbConfig); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal strategy config")
		return
	}

	quotingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.QuotingSymbolUniversalTicker)
	if err != nil {
		log.Error().Err(err).Msg("failed to get quoting symbol")
		return
	}
	hedgingSymbol, err := x.GetCatalog().GetSymbolByUniversalTicker(xarbConfig.HedgingSymbolUniversalTicker)
	if err != nil {
		log.Error().Err(err).Msg("failed to get hedging symbol")
		return
	}
	log.Info().Msgf("Quoting symbol: %s(%d)", quotingSymbol.UniversalTicker, quotingSymbol.ID)
	log.Info().Msgf("Hedging symbol: %s(%d)", hedgingSymbol.UniversalTicker, hedgingSymbol.ID)
	x.quotingSymbol = *quotingSymbol
	x.hedgingSymbol = *hedgingSymbol

	// Initialize IsBookInFlight for both symbols
	x.IsBookInFlight[x.quotingSymbol.ID] = false
	x.IsBookInFlight[x.hedgingSymbol.ID] = false
}

// OnStart subscribes to market data and connects.
func (x *XArb) OnStart() {
	x.SubscribeDepthUpdate(x.quotingSymbol.ID)
	log.Info().Msgf("Subscribed to depth update for quoting symbol: %s(%d)", x.quotingSymbol.UniversalTicker, x.quotingSymbol.ID)
	x.SubscribeDepthUpdate(x.hedgingSymbol.ID)
	log.Info().Msgf("Subscribed to depth update for hedging symbol: %s(%d)", x.hedgingSymbol.UniversalTicker, x.hedgingSymbol.ID)
	x.Connect(context.Background())
}

// OnStop disconnects from market data.
func (x *XArb) OnStop() {
	x.Disconnect()
}

// Handle overrides StrategyBase.Handle to dispatch events to XArb's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (x *XArb) Handle(ev evbus.Event, bus *evbus.EventBus) {
	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		x.OnDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		x.OnDepthUpdate(update)
	case event.DataTypeTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := evbus.DeserializeTick(buf)
		x.OnTick(tick)
	case event.DataTypeOrderUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderUpdate := evbus.DeserializeOrderUpdate(buf)
		x.OnOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		x.OnFill(fill)
	case event.DataTypeReqDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqDepthSnapshot(buf)
		x.OnReqDepthSnapshot(snapshot)
	}
}

// OnDepthSnapshot processes depth snapshots.
func (x *XArb) OnDepthSnapshot(snapshot event.DepthSnapshot) {
	log.Info().Msgf("Depth snapshot: %d %d %d %d", snapshot.SymbolID, snapshot.DepthID, len(snapshot.Bids), len(snapshot.Asks))
}

// OnDepthUpdate processes depth updates.
func (x *XArb) OnDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID

	// Check orderbook state
	bookState, exists := x.GetBookState(symbolID)
	if !exists {
		log.Warn().Int("symbolID", symbolID).Msg("Orderbook not registered")
		return
	}

	// If waiting for snapshot and not already in flight, request snapshot
	if bookState == ob.StateWaitForSnapshot && !x.IsBookInFlight[symbolID] {
		log.Info().Int("symbolID", symbolID).Msg("Requesting depth snapshot (orderbook waiting)")
		if err := x.ReqDepthSnapshot(symbolID); err != nil {
			log.Error().Err(err).Int("symbolID", symbolID).Msg("Failed to request depth snapshot")
		} else {
			x.IsBookInFlight[symbolID] = true
		}
		return
	}

	// If ready, print top 5 levels
	if x.IsSymbolReady(symbolID) {
		x.printTop5(symbolID)
	} else {
		log.Debug().
			Int("symbolID", symbolID).
			Str("state", bookState.String()).
			Bool("inFlight", x.IsBookInFlight[symbolID]).
			Msg("Depth update received, orderbook not ready")
	}
}

// OnReqDepthSnapshot processes the response to a depth snapshot request.
func (x *XArb) OnReqDepthSnapshot(snapshot event.ReqDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log.Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asks", len(snapshot.Asks)).
		Int("bids", len(snapshot.Bids)).
		Msg("ReqDepthSnapshot received")

	// Clear in-flight flag
	x.IsBookInFlight[symbolID] = false
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
	// log.Info().Msg(sb.String())
}

// OnTick processes tick events.
func (x *XArb) OnTick(tick event.Tick) {}

// OnOrderUpdate processes order updates.
func (x *XArb) OnOrderUpdate(update event.OrderUpdate) {}

// OnFill processes fill events.
func (x *XArb) OnFill(fill event.Fill) {}
