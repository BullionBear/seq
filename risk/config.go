package risk

import "github.com/BullionBear/seq/core/actor"

// Config contains risk engine configuration.
type Config struct {
	Actor   []actor.Entry  `yaml:"actor"`
	Checker []CheckerEntry `yaml:"checker"`
}

// CheckerEntry describes a single rule to add to the Checker.
type CheckerEntry struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}
