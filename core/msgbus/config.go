package msgbus

import "github.com/BullionBear/seq/core/logger/rotate"

// Config contains msgbus configuration.
type Config struct {
	MsgLog MsgLogConfig `yaml:"msglog"`
}

// MsgLogConfig contains plaintext event/command JSONL logging configuration.
// Msglog never writes to stdout.
type MsgLogConfig struct {
	Enabled bool              `yaml:"enabled"`
	File    rotate.FileConfig `yaml:"file"`
}
