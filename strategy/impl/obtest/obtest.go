package obtest

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

// Ensure OBTest implements the Actor interface
var _ actor.Actor = (*OBTest)(nil)

// OBTest is a debugging strategy that subscribes to one orderbook and prints debug messages.
type OBTest struct {
	*strategy.StrategyBase // Embed StrategyBase for Actor + StrategyCommon
	symbol                 cpanel.Symbol

	// IsBookInFlight tracks whether a snapshot has been requested but not yet received
	IsBookInFlight bool

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
		StrategyBase:   strategy.NewStrategyBase("obtest"),
		symbol:         cpanel.Symbol{},
		IsBookInFlight: false,
		updateCount:    0,
	}
}

// OnInit initializes the strategy with configuration.
func (o *OBTest) OnInit() {
	// Get strategy-specific config from StrategyBase
	strategyConfig := o.StrategyConfig()
	if strategyConfig == nil {
		log.Error().Msg("strategy config is nil")
		return
	}

	// Convert map[string]any to OBTestConfig struct via YAML re-marshaling
	var obtestConfig OBTestConfig
	yamlData, err := yaml.Marshal(strategyConfig)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal strategy config")
		return
	}
	if err := yaml.Unmarshal(yamlData, &obtestConfig); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal strategy config")
		return
	}

	symbol, err := o.GetCatalog().GetSymbolByUniversalTicker(obtestConfig.SymbolUniversalTicker)
	if err != nil {
		log.Error().Err(err).Msg("failed to get symbol")
		return
	}
	log.Info().Msgf("OBTest: Symbol configured: %s (ID: %d)", symbol.UniversalTicker, symbol.ID)
	o.symbol = *symbol
}

// OnStart subscribes to market data and connects.
func (o *OBTest) OnStart() {
	o.SubscribeDepthUpdate(o.symbol.ID)
	log.Info().Msgf("OBTest: Subscribed to depth update for symbol: %s (ID: %d)", o.symbol.UniversalTicker, o.symbol.ID)
	o.Connect(context.Background())
	log.Info().Msg("OBTest: Connected to market data")
}

// OnStop disconnects from market data.
func (o *OBTest) OnStop() {
	o.Disconnect()
	log.Info().Msg("OBTest: Disconnected from market data")
}

// Handle overrides StrategyBase.Handle to dispatch events to OBTest's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (o *OBTest) Handle(ev evbus.Event, bus *evbus.EventBus) {
	// Log ALL incoming events at the top level for debugging
	log.Debug().
		Int("dataType", int(ev.Ref.DataType)).
		Uint64("eventID", ev.EventID).
		Msgf("OBTest: Handle called with event type: %d", ev.Ref.DataType)

	switch ev.Ref.DataType {
	case event.DataTypeDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeDepthSnapshot(buf)
		o.OnDepthSnapshot(snapshot)
	case event.DataTypeDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := evbus.DeserializeDepthUpdate(buf)
		o.OnDepthUpdate(update)
	case event.DataTypeTick:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		tick := evbus.DeserializeTick(buf)
		o.OnTick(tick)
	case event.DataTypeOrderUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderUpdate := evbus.DeserializeOrderUpdate(buf)
		o.OnOrderUpdate(orderUpdate)
	case event.DataTypeFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := evbus.DeserializeFill(buf)
		o.OnFill(fill)
	case event.DataTypeReqDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := evbus.DeserializeReqDepthSnapshot(buf)
		o.OnReqDepthSnapshot(snapshot)
	default:
		log.Warn().Int("dataType", int(ev.Ref.DataType)).Msg("OBTest: Unknown event type")
	}
}

// OnDepthSnapshot processes depth snapshots.
func (o *OBTest) OnDepthSnapshot(snapshot event.DepthSnapshot) {
	o.snapshotDepthID = snapshot.DepthID

	log.Info().
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
		log.Debug().
			Float64("bestBidPrice", snapshot.Bids[0].Price).
			Float64("bestBidQty", snapshot.Bids[0].Quantity).
			Msg("OBTest: Best bid from snapshot")
	}
	if len(snapshot.Asks) > 0 {
		log.Debug().
			Float64("bestAskPrice", snapshot.Asks[0].Price).
			Float64("bestAskQty", snapshot.Asks[0].Quantity).
			Msg("OBTest: Best ask from snapshot")
	}
}

// OnDepthUpdate processes depth updates.
func (o *OBTest) OnDepthUpdate(update event.DepthUpdate) {
	symbolID := update.SymbolID
	o.updateCount++

	// Track sequence gap
	expectedPrevID := o.lastReceivedDepthID
	receivedPrevID := update.PreviousDepthID
	receivedDepthID := update.DepthID
	gapFromLastReceived := receivedPrevID - expectedPrevID
	depthIDJump := receivedDepthID - receivedPrevID

	// Check orderbook state
	bookState, exists := o.GetBookState(symbolID)
	if !exists {
		log.Warn().Int("symbolID", symbolID).Msg("OBTest: Orderbook not registered")
		return
	}

	// Log every depth update with detailed sequence tracking
	log.Info().
		Int("symbolID", symbolID).
		Int("depthID", receivedDepthID).
		Int("prevDepthID", receivedPrevID).
		Int("lastReceivedDepthID", expectedPrevID).
		Int("snapshotDepthID", o.snapshotDepthID).
		Int("gapFromLastReceived", gapFromLastReceived).
		Int("depthIDJump", depthIDJump).
		Int("bidsCount", len(update.Bids)).
		Int("asksCount", len(update.Asks)).
		Str("bookState", bookState.String()).
		Int("updateCount", o.updateCount).
		Msg("OBTest: Depth update received")

	// Log gap information (gaps are expected for 100ms aggregated streams)
	// Only warn if the gap is unusually large (> 100 depthIDs suggests potential issue)
	if expectedPrevID > 0 && gapFromLastReceived != 0 {
		if gapFromLastReceived > 100 || gapFromLastReceived < 0 {
			log.Warn().
				Int("expectedPrevID", expectedPrevID).
				Int("receivedPrevID", receivedPrevID).
				Int("gap", gapFromLastReceived).
				Msg("OBTest: Large gap detected - may indicate missed updates")
		} else {
			log.Debug().
				Int("expectedPrevID", expectedPrevID).
				Int("receivedPrevID", receivedPrevID).
				Int("gap", gapFromLastReceived).
				Msg("OBTest: Normal aggregation gap (expected for 100ms stream)")
		}
	}

	// Update tracking
	o.lastReceivedPrevID = receivedPrevID
	o.lastReceivedDepthID = receivedDepthID

	// If waiting for snapshot and not already in flight, request snapshot
	if bookState == ob.StateWaitForSnapshot && !o.IsBookInFlight {
		log.Info().Int("symbolID", symbolID).Msg("OBTest: Requesting depth snapshot (orderbook waiting)")
		if err := o.ReqDepthSnapshot(symbolID); err != nil {
			log.Error().Err(err).Int("symbolID", symbolID).Msg("OBTest: Failed to request depth snapshot")
		} else {
			o.IsBookInFlight = true
		}
		return
	}

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
		log.Debug().
			Int("symbolID", symbolID).
			Str("state", bookState.String()).
			Bool("inFlight", o.IsBookInFlight).
			Msg("OBTest: Depth update received, orderbook not ready")
	}
}

// OnReqDepthSnapshot processes the response to a depth snapshot request.
func (o *OBTest) OnReqDepthSnapshot(snapshot event.ReqDepthSnapshot) {
	symbolID := snapshot.SymbolID
	log.Info().
		Int("symbolID", symbolID).
		Int("depthID", snapshot.DepthID).
		Int("asksCount", len(snapshot.Asks)).
		Int("bidsCount", len(snapshot.Bids)).
		Msg("OBTest: ReqDepthSnapshot received")

	// Clear in-flight flag
	o.IsBookInFlight = false
}

// printSummary prints a brief summary of the orderbook state.
func (o *OBTest) printSummary(symbolID int) {
	bestBid, bidQty, bidOk := o.GetBestBid(symbolID)
	bestAsk, askQty, askOk := o.GetBestAsk(symbolID)
	midPrice, midOk := o.GetMidPrice(symbolID)
	spread, spreadOk := o.GetSpread(symbolID)

	if bidOk && askOk && midOk && spreadOk {
		log.Info().
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
	log.Info().Msg(sb.String())
}

// OnTick processes tick events.
func (o *OBTest) OnTick(tick event.Tick) {
	log.Debug().
		Int("symbolID", tick.SymbolID).
		Float64("price", tick.Price).
		Float64("qty", tick.Qty).
		Msg("OBTest: Tick received")
}

// OnOrderUpdate processes order updates.
func (o *OBTest) OnOrderUpdate(update event.OrderUpdate) {
	log.Debug().
		Int("orderID", update.OrderID).
		Int("clientOrderID", update.ClientOrderID).
		Int("status", int(update.OrderStatus)).
		Float64("executedQty", update.ExecutedQty).
		Msg("OBTest: Order update received")
}

// OnFill processes fill events.
func (o *OBTest) OnFill(fill event.Fill) {
	log.Debug().
		Int("orderID", fill.OrderID).
		Int("clientOrderID", fill.ClientOrderID).
		Float64("filledPrice", fill.FilledPrice).
		Float64("filledQty", fill.FilledQty).
		Msg("OBTest: Fill received")
}
