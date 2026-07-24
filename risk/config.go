package risk

import "github.com/BullionBear/seq/core/actor"

// Config contains risk engine configuration.
type Config struct {
	Actor []actor.Entry `yaml:"actor"`
}
