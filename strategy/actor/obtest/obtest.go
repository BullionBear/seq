package obtest

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/strategy"
	"github.com/mitchellh/mapstructure"
)

func init() {
	strategy.Register("obtest", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewOBTest(cat, c, bus)
	})
}

// Ensure OBTest implements the Actor interface
var _ actor.Actor = (*OBTest)(nil)

// OBTest is a debugging strategy that subscribes to one orderbook and prints debug messages.
type OBTest struct {
	strategy.StrategyActorBase // Embed StrategyBase for Actor + StrategyCommon
	symbol                     catalog.Symbol

	// Counter for update messages to avoid flooding logs
	updateCount int

	// Sequence tracking for debugging
	lastReceivedDepthID int
	lastReceivedPrevID  int
	snapshotDepthID     int
}

// NewOBTest creates a new OBTest strategy.
func NewOBTest(cat *catalog.Catalog, cache *cache.Cache, msgbus *msgbus.MsgBus) *OBTest {
	return &OBTest{
		StrategyActorBase: strategy.NewStrategyActorBase("obtest", cat, cache, msgbus, []event.Topic{
			event.TopicEventDepthSnapshot,
			event.TopicEventDepthUpdate,
			event.TopicEventRespDepthSnapshot,
		}),
		symbol:      catalog.Symbol{},
		updateCount: 0,
	}
}

// OnInit initializes the strategy with configuration.
func (o *OBTest) OnInit(config map[string]any) {
	// Get strategy-specific config from StrategyBase
	var obtestConfig OBTestConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &obtestConfig,
		TagName: "yaml", // Use yaml tags for mapping
	})
	if err != nil {
		o.Log().Panic().Msg("failed to create decoder")
		return
	}

	err = decoder.Decode(config)
	if err != nil {
		o.Log().Panic().Msg("failed to decode config")
		return
	}

	symbol, err := o.GetCatalog().GetSymbolByUniversalTicker(obtestConfig.SymbolUniversalTicker)
	if err != nil {
		o.Log().Error().Err(err).Msg("failed to get symbol")
		return
	}
	o.Log().Info().Msgf("OBTest: Symbol configured: %s (ID: %d)", symbol.UniversalTicker, symbol.ID)
	o.symbol = *symbol
}

// OnStart is called when the strategy starts.
// Note: Data subscriptions are now handled by the config - no manual Subscribe/Connect needed.
func (o *OBTest) OnStart() {
	o.Log().Info().Msgf("OBTest: Strategy started for symbol: %s (ID: %d)", o.symbol.UniversalTicker, o.symbol.ID)
}

// OnStop is called when the strategy stops.
func (o *OBTest) OnStop() {
	o.Log().Info().Msg("OBTest: Strategy stopped")
}

// Handle overrides StrategyBase.Handle to dispatch events to OBTest's typed callbacks.
// This is necessary because Go doesn't have virtual method dispatch.
func (o *OBTest) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	// Log ALL incoming events at the top level for debugging
	o.Log().Debug().
		Int("topic", int(ev.Ref.Topic)).
		Uint64("eventID", ev.EventID).
		Msgf("OBTest: Handle called with topic: %d", ev.Ref.Topic)

	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot, err := event.NewDepthSnapshotFromBytes(buf)
		if err != nil {
			return
		}
		o.OnDepthSnapshot(snapshot)
	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update, err := event.NewDepthUpdateFromBytes(buf)
		if err != nil {
			return
		}
		o.OnDepthUpdate(update)
	case event.TopicEventRespDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot, err := event.NewRespDepthSnapshotFromBytes(buf)
		if err != nil {
			return
		}
		o.OnRespDepthSnapshot(snapshot)
	default:
		o.Log().Warn().Int("topic", int(ev.Ref.Topic)).Msg("OBTest: Unknown topic")
	}
}

// OnDepthSnapshot processes depth snapshots.
func (o *OBTest) OnDepthSnapshot(snapshot event.DepthSnapshot) {
}

// OnDepthUpdate processes depth updates.
// Note: Snapshot requests are now handled automatically by DataEngine.
func (o *OBTest) OnDepthUpdate(update event.DepthUpdate) {
}

// OnReqDepthSnapshot processes the response to a depth snapshot request.
func (o *OBTest) OnRespDepthSnapshot(snapshot event.RespDepthSnapshot) {

}
