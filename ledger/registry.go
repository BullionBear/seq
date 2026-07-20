package ledger

import (
	"fmt"
	"sync"

	"github.com/BullionBear/seq/core/actor"
)

// Factory is a constructor function that creates a ledger actor.
// The handler parameter is the ledger engine; each actor package
// defines its own interface and performs a type assertion.
type Factory func(handler any) actor.Actor

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register registers a ledger actor factory under the given type name.
// Typically called from init() in each ledger actor package.
func Register(typeName string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[typeName]; dup {
		panic(fmt.Sprintf("ledger: Register called twice for type %q", typeName))
	}
	registry[typeName] = factory
}

func lookupFactory(typeName string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[typeName]
	if !ok {
		return nil, fmt.Errorf("ledger: unknown actor type %q", typeName)
	}
	return f, nil
}
