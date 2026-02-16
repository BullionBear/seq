package portfolio

import "github.com/BullionBear/seq/core/actor"

// Config contains portfolio engine configuration.
type Config struct {
	Actor []actor.Entry `yaml:"actor"`
}
