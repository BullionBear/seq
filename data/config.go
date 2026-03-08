package data

import "github.com/BullionBear/seq/core/actor"

// Config contains data engine configuration.
type Config struct {
	Actor []actor.Entry `yaml:"actor"`
}
