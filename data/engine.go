package data

import (
	"context"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/engine"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/data/actor/orderbook"
	"github.com/rs/zerolog"
)

// Ensure Engine implements the EngineService interface
var _ engine.Engine = (*Engine)(nil)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// DataSubscription holds parsed subscription config for a single symbol
type DataSubscription struct {
	SymbolID int
	Endpoint string                // regional endpoint override
	Depth    *adapter.DepthOptions // nil if no depth subscription
	Trade    *adapter.TradeOptions // nil if no trade subscription
}

// Engine manages market data processing including orderbook management.
// It owns the OrderBook actor and registers it with the EventBus.
// The engine itself is NOT an actor.
type Engine struct {
	engine.EngineBase
	catalog   *catalog.Catalog
	msgBus    *msgbus.MsgBus
	orderBook *orderbook.Actor
	router    *adapter.DataRouter

	// Data subscriptions from config
	dataSubs []DataSubscription
}

// NewEngine creates a new data engine.
func NewEngine(cat *catalog.Catalog, msgBus *msgbus.MsgBus, c *cache.Cache) *Engine {
	router := adapter.NewDataRouter(cat, msgBus)
	return &Engine{
		EngineBase: engine.NewEngineBase(common.EngineData),
		catalog:    cat,
		msgBus:     msgBus,
		orderBook:  orderbook.NewActor(c),
		router:     router,
	}
}

// handledCommandTypes returns the command types this engine processes.
func (e *Engine) handledCommandTypes() []command.CommandType {
	return []command.CommandType{
		command.CommandTypeReqDepthSnapshot,
	}
}

// Init registers the engine's actors and command processors with the MsgBus.
func (e *Engine) Init() {
	actor.Register(e.msgBus, e.orderBook)
	for _, cmdType := range e.handledCommandTypes() {
		cmdType := cmdType
		e.msgBus.RegisterCommand(cmdType, func(cmd msgbus.Command) { e.Execute(cmd, e.msgBus) })
	}
	log().Info().Msg("DataEngine initialized")
}

func (e *Engine) Start() {
	e.Connect(context.Background())
	e.orderBook.OnStart()
	log().Info().Msg("DataEngine started")
	e.NotifyReady()
}

func (e *Engine) Stop() {
	e.Disconnect()
	e.orderBook.OnStop()
	log().Info().Msg("DataEngine stopped")
	e.NotifyStop()
}

// Execute routes commands to the appropriate handler.
func (e *Engine) Execute(cmd msgbus.Command, bus *msgbus.MsgBus) {
	switch cmd.Ref.CommandType {
	case command.CommandTypeReqDepthSnapshot:
		buf := bus.ReadCmdBuffer(cmd.Ref.Index, cmd.Ref.Length)
		req := command.NewReqDepthSnapshotFromBytes(buf)
		e.execReqDepthSnapshot(req)
	}
}

func (e *Engine) execReqDepthSnapshot(req command.ReqDepthSnapshot) {
	e.router.ReqDepthSnapshot(req.SymbolID)
}

// ============================================================================
// Config-based Subscription Methods
// ============================================================================

// SetDataConfig parses config and stores subscription requirements.
// Call this before Connect() to configure what data to subscribe to.
func (e *Engine) SetDataConfig(config Config) error {
	e.dataSubs = make([]DataSubscription, 0, len(config.Subscriptions))

	for _, cfg := range config.Subscriptions {
		symbol, err := e.catalog.GetSymbolByUniversalTicker(cfg.Symbol)
		if err != nil {
			log().Error().Err(err).Str("symbol", cfg.Symbol).Msg("DataEngine: Failed to resolve symbol from config")
			return err
		}

		sub := DataSubscription{
			SymbolID: symbol.ID,
			Endpoint: cfg.Endpoint,
		}

		// Parse depth options
		if cfg.Depth != nil {
			sub.Depth = &adapter.DepthOptions{
				Type:     cfg.Depth.Type,
				PushRate: cfg.Depth.PushRate,
				Levels:   cfg.Depth.Levels,
			}
		}

		// Parse trade options
		if cfg.Trade != nil {
			sub.Trade = &adapter.TradeOptions{
				Type: cfg.Trade.Type,
			}
		}

		e.dataSubs = append(e.dataSubs, sub)
		log().Debug().
			Int("symbolID", symbol.ID).
			Str("ticker", cfg.Symbol).
			Bool("hasDepth", sub.Depth != nil).
			Bool("hasTrade", sub.Trade != nil).
			Msg("DataEngine: Configured subscription")
	}

	return nil
}

// ============================================================================
// Legacy Subscription Methods (for backward compatibility)
// ============================================================================

// SubscribeDepthUpdate subscribes to depth updates for a symbol.
// Deprecated: Use SetDataConfig() instead.
func (e *Engine) SubscribeDepthUpdate(symbolID int) {
	// Get symbol from catalog to get price precision
	symbol, err := e.catalog.GetSymbol(symbolID)
	if err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to get symbol from catalog")
		return
	}

	// Register symbol with orderbook actor
	e.orderBook.RegisterSymbol(symbolID, symbol.PricePrecision, symbol.SizePrecision)

	// Subscribe to depth updates via router with default options
	if err := e.router.SubscribeDepthUpdate(symbolID, nil); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to depth update")
	}
}

// SubscribeTick subscribes to tick updates for a symbol.
// Deprecated: Use SetDataConfig() instead.
func (e *Engine) SubscribeTick(symbolID int) {
	if err := e.router.SubscribeTrade(symbolID, nil); err != nil {
		log().Error().Err(err).Int("symbolID", symbolID).Msg("DataEngine: Failed to subscribe to tick")
	}
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect subscribes to all configured data streams and connects to data sources.
func (e *Engine) Connect(ctx context.Context) {
	// Subscribe to all configured data streams
	for _, sub := range e.dataSubs {
		// Get symbol for orderbook registration
		symbol, err := e.catalog.GetSymbol(sub.SymbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to get symbol")
			continue
		}

		// Subscribe to depth if configured
		if sub.Depth != nil {
			// Register symbol with orderbook actor
			e.orderBook.RegisterSymbol(sub.SymbolID, symbol.PricePrecision, symbol.SizePrecision)

			// Subscribe via router with options
			if err := e.router.SubscribeDepthUpdate(sub.SymbolID, sub.Depth); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to depth")
			}
		}

		// Subscribe to trade if configured
		if sub.Trade != nil {
			if err := e.router.SubscribeTrade(sub.SymbolID, sub.Trade); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to trade")
			}
		}
	}

	// Connect router (which connects exchange clients)
	if err := e.router.Connect(ctx); err != nil {
		log().Error().Err(err).Msg("DataEngine: Failed to connect to data router")
	}
}

// Disconnect disconnects from all data sources.
func (e *Engine) Disconnect() {
	e.router.Disconnect()
}
