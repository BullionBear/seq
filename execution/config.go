package execution

import "github.com/BullionBear/seq/core/actor"

// Config contains execution engine configuration.
type Config struct {
	Actor []actor.Entry `yaml:"actor"`
}
