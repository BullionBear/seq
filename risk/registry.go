package risk

import (
	"fmt"
	"sync"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/msgbus"
)

// Factory is a constructor function that creates a risk actor.
type Factory func(bus *msgbus.MsgBus, c *cache.Cache) actor.Actor

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register registers a risk actor factory under the given type name.
// Typically called from init() in each risk actor package.
func Register(typeName string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[typeName]; dup {
		panic(fmt.Sprintf("risk: Register called twice for type %q", typeName))
	}
	registry[typeName] = factory
}

func lookupFactory(typeName string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[typeName]
	if !ok {
		return nil, fmt.Errorf("risk: unknown actor type %q", typeName)
	}
	return f, nil
}
