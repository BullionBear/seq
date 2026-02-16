package strategy

import (
	"fmt"
	"sync"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/msgbus"
)

// Config contains strategy engine configuration.
type Config []Entry

// Entry defines a strategy actor to instantiate from config.
type Entry struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}

// Factory is a constructor function that creates a strategy actor.
type Factory func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register registers a strategy actor factory under the given type name.
// Typically called from init() in each strategy actor package.
func Register(typeName string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[typeName]; dup {
		panic(fmt.Sprintf("strategy: Register called twice for type %q", typeName))
	}
	registry[typeName] = factory
}

// lookupFactory returns the factory for the given type name.
func lookupFactory(typeName string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[typeName]
	if !ok {
		return nil, fmt.Errorf("strategy: unknown actor type %q", typeName)
	}
	return f, nil
}
