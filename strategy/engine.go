package strategy

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

// Engine manages the lifecycle of multiple strategy actors.
// It constructs actors from config entries using the factory registry,
// registers them with the MsgBus, and manages their lifecycle.
type Engine struct {
	actors  []actor.Actor
	catalog *catalog.Catalog
	cache   *cache.Cache
	msgBus  *msgbus.MsgBus
}

// NewEngine creates a new strategy Engine.
func NewEngine(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) *Engine {
	return &Engine{
		catalog: cat,
		cache:   c,
		msgBus:  bus,
	}
}

// Init constructs strategy actors from config entries using the factory
// registry, registers them with the MsgBus, and calls OnInit.
func (e *Engine) Init(config Config) {
	for _, entry := range config.Actor {
		factory, err := lookupFactory(entry.Type)
		if err != nil {
			log().Error().Err(err).Str("type", entry.Type).Msg("StrategyEngine: skipping unknown strategy type")
			continue
		}

		a := factory(e.catalog, e.msgBus, e.cache)
		actor.ApplyName(a, entry.Name)
		actor.Register(e.msgBus, a)
		a.OnInit(entry.Config)
		e.actors = append(e.actors, a)

		log().Info().Str("type", entry.Type).Str("name", a.Name()).Msg("StrategyEngine: actor initialized")
	}
}

// Start calls OnStart on every actor.
func (e *Engine) Start() {
	for _, a := range e.actors {
		a.OnStart()
	}
}

// Stop calls OnStop on every actor.
func (e *Engine) Stop() {
	for _, a := range e.actors {
		a.OnStop()
	}
}
