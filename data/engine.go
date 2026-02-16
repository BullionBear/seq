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

// SymbolRegistrar is an optional interface that data actors can implement
// to receive symbol registration calls from the engine during Connect.
type SymbolRegistrar interface {
	RegisterSymbol(symbolID, pricePrecision, sizePrecision int)
}

// Engine manages market data processing including orderbook management.
// It constructs actors from config via the factory registry.
type Engine struct {
	engine.EngineBase
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus
	cache   *cache.Cache
	router  *adapter.DataRouter
	actors  []actor.Actor

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
		cache:      c,
		router:     router,
	}
}

// handledCommandTypes returns the command types this engine processes.
func (e *Engine) handledCommandTypes() []command.CommandType {
	return []command.CommandType{
		command.CommandTypeReqDepthSnapshot,
	}
}

// Init constructs actors from config, registers them and command processors.
func (e *Engine) Init(config Config) {
	// Parse data subscriptions
	e.parseSubscriptions(config)

	// Construct actors from config entries
	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			log().Error().Err(err).Str("type", entry.Type).Msg("DataEngine: skipping unknown actor type")
			continue
		}

		a := factory(e.catalog, e.msgBus, e.cache)
		actor.Register(e.msgBus, a)
		a.OnInit(entry.Config)
		e.actors = append(e.actors, a)

		log().Info().Str("type", entry.Type).Str("name", a.Name()).Msg("DataEngine: actor initialized")
	}

	// Register command processors
	for _, cmdType := range e.handledCommandTypes() {
		cmdType := cmdType
		e.msgBus.RegisterCommand(cmdType, func(cmd msgbus.Command) { e.Execute(cmd, e.msgBus) })
	}

	log().Info().Msg("DataEngine initialized")
}

func (e *Engine) Start() {
	e.Connect(context.Background())
	for _, a := range e.actors {
		a.OnStart()
	}
	log().Info().Msg("DataEngine started")
	e.NotifyReady()
}

func (e *Engine) Stop() {
	e.Disconnect()
	for _, a := range e.actors {
		a.OnStop()
	}
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
// Config Parsing
// ============================================================================

func (e *Engine) parseSubscriptions(config Config) {
	e.dataSubs = make([]DataSubscription, 0, len(config.Subscriptions))

	for _, cfg := range config.Subscriptions {
		symbol, err := e.catalog.GetSymbolByUniversalTicker(cfg.Symbol)
		if err != nil {
			log().Error().Err(err).Str("symbol", cfg.Symbol).Msg("DataEngine: Failed to resolve symbol from config")
			continue
		}

		sub := DataSubscription{
			SymbolID: symbol.ID,
			Endpoint: cfg.Endpoint,
		}

		if cfg.Depth != nil {
			sub.Depth = &adapter.DepthOptions{
				Type:     cfg.Depth.Type,
				PushRate: cfg.Depth.PushRate,
				Levels:   cfg.Depth.Levels,
			}
		}

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
}

// ============================================================================
// Connection Methods
// ============================================================================

// Connect subscribes to all configured data streams and connects to data sources.
func (e *Engine) Connect(ctx context.Context) {
	for _, sub := range e.dataSubs {
		symbol, err := e.catalog.GetSymbol(sub.SymbolID)
		if err != nil {
			log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to get symbol")
			continue
		}

		if sub.Depth != nil {
			// Notify all actors that support symbol registration
			for _, a := range e.actors {
				if sr, ok := a.(SymbolRegistrar); ok {
					sr.RegisterSymbol(sub.SymbolID, symbol.PricePrecision, symbol.SizePrecision)
				}
			}

			if err := e.router.SubscribeDepthUpdate(sub.SymbolID, sub.Depth); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to depth")
			}
		}

		if sub.Trade != nil {
			if err := e.router.SubscribeTrade(sub.SymbolID, sub.Trade); err != nil {
				log().Error().Err(err).Int("symbolID", sub.SymbolID).Msg("DataEngine: Failed to subscribe to trade")
			}
		}
	}

	if err := e.router.Connect(ctx); err != nil {
		log().Error().Err(err).Msg("DataEngine: Failed to connect to data router")
	}
}

// Disconnect disconnects from all data sources.
func (e *Engine) Disconnect() {
	e.router.Disconnect()
}
