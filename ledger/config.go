package ledger

import "github.com/BullionBear/seq/core/actor"

// Config contains ledger engine configuration.
type Config struct {
	Actor []actor.Entry `yaml:"actor"`
}
