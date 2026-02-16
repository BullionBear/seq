package portfolio

import (
	"fmt"
	"sync"

	"github.com/BullionBear/seq/core/actor"
)

// Factory is a constructor function that creates a portfolio actor.
// The handler parameter is the portfolio engine (implements BalanceEngineHandler).
type Factory func(handler BalanceEngineHandler) actor.Actor

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register registers a portfolio actor factory under the given type name.
// Typically called from init() in each portfolio actor package.
func Register(typeName string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[typeName]; dup {
		panic(fmt.Sprintf("portfolio: Register called twice for type %q", typeName))
	}
	registry[typeName] = factory
}

func lookupFactory(typeName string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[typeName]
	if !ok {
		return nil, fmt.Errorf("portfolio: unknown actor type %q", typeName)
	}
	return f, nil
}
