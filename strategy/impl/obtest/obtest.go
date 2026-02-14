package obtest

import (
	"fmt"
	"strings"
	"time"

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

// Ensure OBTest implements the Actor interface
var _ actor.Actor = (*OBTest)(nil)

// OBTest is a debugging strategy that subscribes to one orderbook and prints debug messages.
type OBTest struct {
	*strategy.StrategyBase // Embed StrategyBase for Actor + StrategyCommon
	symbol                 cpanel.Symbol

	// Counter for update messages to avoid flooding logs
	updateCount int

	// Sequence tracking for debugging
	lastReceivedDepthID int
	lastReceivedPrevID  int
	snapshotDepthID     int
}

// NewOBTest creates a new OBTest strategy.
func NewOBTest() *OBTest {
	return &OBTest{
		StrategyBase: strategy.NewStrategyBase("obtest"),
		symbol:       cpanel.Symbol{},
		updateCount:  0,
	}
}

// OnInit initializes the strategy with configuration.
func (o *OBTest) OnInit() {
	// Get strategy-specific config from StrategyBase
	strategyConfig := o.StrategyConfig()
	if strategyConfig == nil {
		log().Error().Msg("strategy config is nil")
		return
	}

	// Convert map[string]any to OBTestConfig struct via YAML re-marshaling
	var obtestConfig OBTestConfig
	yamlData, err := yaml.Marshal(strategyConfig)
	if err != nil {
		log().Error().Err(err).Msg("failed to marshal strategy config")
		return
	}
	if err := yaml.Unmarshal(yamlData, &obtestConfig); err != nil {
		log().Error().Err(err).Msg("failed to unmarshal strategy config")
		return
	}

	symbol, err := o.GetCatalog().GetSymbolByUniversalTicker(obtestConfig.SymbolUniversalTicker)
	if err != nil {
		log().Error().Err(err).Msg("failed to get symbol")
		return
	}
	log().Info().Msgf("OBTest: Symbol configured: %s (ID: %d)", symbol.UniversalTicker, symbol.ID)
	o.symbol = *symbol
}

// OnStart is called when the strategy starts.
// Note: Data subscriptions are now handled by the config - no manual Subscribe/Connect needed.
func (o *OBTest) OnStart() {
	log().Info().Msgf("OBTest: Strategy started for symbol: %s (ID: %d)", o.symbol.UniversalTicker, o.symbol.ID)
}

// OnStop is called when the strategy stops.
func (o *OBTest) OnStop() {
	log().Info().Msg("OBTest: Strategy stopped")
}

// Handle overrides StrategyBase.Handle to dispatch events to OBTest's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (o *OBTest) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	// Log ALL incoming events at the top level for debugging
	log().Debug().
		Int("topic", int(ev.Ref.Topic)).
		Uint64("eventID", ev.EventID).
		Msgf("OBTest: Handle called with topic: %d", ev.Ref.Topic)

	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := msgbus.DeserializeDepthSnapshot(buf)
		o.OnDepthSnapshot(snapshot)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := msgbus.DeserializeDepthUpdate(buf)
		o.OnDepthUpdate(update)
	case event.TopicEventTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := msgbus.DeserializeTick(buf)
		o.OnTick(tick)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := msgbus.DeserializeFill(buf)
		o.OnFill(fill)
	case event.TopicEventRespDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := msgbus.DeserializeRespDepthSnapshot(buf)
		o.OnRespDepthSnapshot(snapshot)
	default:
		log().Warn().Int("topic", int(ev.Ref.Topic)).Msg("OBTest: Unknown topic")
	}
}

// OnDepthSnapshot processes depth snapshots.
func (o *OBTest) OnDepthSnapshot(snapshot event.DepthSnapshot) {
	o.snapshotDepthID = snapshot.DepthID

	log().Info().
		Int("symbolID", snapshot.SymbolID).
		Int("depthID", snapshot.DepthID).
		Int("bidsCount", len(snapshot.Bids)).
		Int("asksCount", len(snapshot.Asks)).
		Uint64("timestamp", snapshot.Timestamp).
		Int("lastReceivedDepthID", o.lastReceivedDepthID).
		Int("lastReceivedPrevID", o.lastReceivedPrevID).
		Msg("OBTest: Depth snapshot received")

	// Print first few bid/ask levels
	if len(snapshot.Bids) > 0 {
		log().Debug().
			Float64("bestBidPrice", snapshot.Bids[0].Price).
			Float64("bestBidQty", snapshot.Bids[0].Quantity).
			Msg("OBTest: Best bid from snapshot")
	}
	if len(snapshot.Asks) > 0 {
		log().Debug().
			Float64("bestAskPrice", snapshot.Asks[0].Price).
			Float64("bestAskQty", snapshot.Asks[0].Quantity).
			Msg("OBTest: Best ask from snapshot")
	}
}

// OnDepthUpdate processes depth updates.
// Note: Snapshot requests are now handled automatically by DataEngine.
func (o *OBTest) OnDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID
	o.updateCount++

	// Track sequence using the correct fields:
	// - PreviousDepthID = U - 1 (one before first update ID)
	// - DepthID = U (first update ID in event)
	// - CurrentDepthID = u (final update ID in event)
	// For consecutive updates: prev_u == current_U - 1, so prev_u == current_PreviousDepthID
	expectedPrevID := o.lastReceivedDepthID  // This should be prev update's final ID (u)
	receivedPrevID := update.PreviousDepthID // Current update's U - 1
	receivedFirstID := update.DepthID        // Current update's U (first update ID)
	receivedFinalID := update.CurrentDepthID // Current update's u (final update ID)
	gapFromLastReceived := receivedPrevID - expectedPrevID
	updateSpan := receivedFinalID - receivedFirstID + 1 // How many individual changes in this update

	// Check orderbook state
	bookState, exists := o.GetBookState(symbolID)
	if !exists {
		log().Warn().Int("symbolID", symbolID).Msg("OBTest: Orderbook not registered")
		return
	}

	// Log every depth update with detailed sequence tracking
	log().Info().
		Int("symbolID", symbolID).
		Int("firstDepthID", receivedFirstID).
		Int("finalDepthID", receivedFinalID).
		Int("prevDepthID", receivedPrevID).
		Int("lastFinalDepthID", expectedPrevID).
		Int("snapshotDepthID", o.snapshotDepthID).
		Int("sequenceGap", gapFromLastReceived).
		Int("updateSpan", updateSpan).
		Int("bidsCount", len(update.Bids)).
		Int("asksCount", len(update.Asks)).
		Str("bookState", bookState.String()).
		Int("updateCount", o.updateCount).
		Msg("OBTest: Depth update received")

	// Log gap information
	// For consecutive updates: gap should be 0 (prev_u == current_U - 1)
	// A gap > 0 means we missed some updates
	if expectedPrevID > 0 && gapFromLastReceived != 0 {
		if gapFromLastReceived > 0 {
			log().Warn().
				Int("lastFinalDepthID", expectedPrevID).
				Int("currentPrevDepthID", receivedPrevID).
				Int("missedUpdates", gapFromLastReceived).
				Msg("OBTest: Sequence gap detected - missed updates")
		} else {
			log().Debug().
				Int("lastFinalDepthID", expectedPrevID).
				Int("currentPrevDepthID", receivedPrevID).
				Int("overlap", -gapFromLastReceived).
				Msg("OBTest: Overlapping update (normal after snapshot)")
		}
	}

	// Update tracking - store the FINAL depth ID (u) for sequence tracking
	o.lastReceivedPrevID = receivedPrevID
	o.lastReceivedDepthID = receivedFinalID // Track final ID (u), not first ID (U)

	// If ready, print orderbook state
	if o.IsSymbolReady(symbolID) {
		// Print detailed orderbook every 10 updates to avoid flooding
		if o.updateCount%10 == 0 {
			o.printOrderbook(symbolID)
		} else {
			// Print summary for every update
			o.printSummary(symbolID)
		}
	} else {
		log().Debug().
			Int("symbolID", symbolID).
			Str("state", bookState.String()).
			Msg("OBTest: Depth update received, orderbook not ready")
	}
}

// OnReqDepthSnapshot processes the response to a depth snapshot request.
func (o *OBTest) OnRespDepthSnapshot(snapshot event.RespDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log().Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asksCount", len(snapshot.Asks)).
		Int("bidsCount", len(snapshot.Bids)).
		Msg("OBTest: RespDepthSnapshot received")
}

// printSummary prints a brief summary of the orderbook state.
func (o *OBTest) printSummary(symbolID int) {
	bestBid, bidQty, bidOk := o.GetBestBid(symbolID)
	bestAsk, askQty, askOk := o.GetBestAsk(symbolID)
	midPrice, midOk := o.GetMidPrice(symbolID)
	spread, spreadOk := o.GetSpread(symbolID)

	if bidOk && askOk && midOk && spreadOk {
		log().Info().
			Int("symbolID", symbolID).
			Float64("bestBid", bestBid).
			Float64("bidQty", bidQty).
			Float64("bestAsk", bestAsk).
			Float64("askQty", askQty).
			Float64("midPrice", midPrice).
			Float64("spread", spread).
			Int("updateCount", o.updateCount).
			Msg("OBTest: Orderbook summary")
	}
}

// printOrderbook prints the top 5 bid and ask levels for a symbol.
func (o *OBTest) printOrderbook(symbolID int) {
	bids, asks := o.GetDepth(symbolID, 5)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n=== OBTest Orderbook [Symbol %d] Update #%d ===\n", symbolID, o.updateCount))
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
	log().Info().Msg(sb.String())
}

// OnTick processes tick events.
func (o *OBTest) OnTick(tick event.Tick) {
	log().Debug().
		Int("symbolID", tick.SymbolID).
		Float64("price", tick.Price).
		Float64("qty", tick.Qty).
		Msg("OBTest: Tick received")
}

// OnFill processes fill events.
func (o *OBTest) OnFill(fill event.Fill) {
	log().Debug().
		Int("orderID", fill.OrderID).
		Int("clientOrderID", fill.ClientOrderID).
		Float64("filledPrice", fill.FilledPrice).
		Float64("filledQty", fill.FilledQty).
		Msg("OBTest: Fill received")
}
