package logger

import "github.com/BullionBear/seq/core/logger/rotate"

// Config contains logger configuration.
type Config struct {
	Level  string            `yaml:"level"`
	Stdout *bool             `yaml:"stdout"` // nil defaults to true
	File   rotate.FileConfig `yaml:"file"`   // empty Dir = no file output
}

// ToOptions converts a Config to an Options struct for Init().
func (c Config) ToOptions() Options {
	stdout := true
	if c.Stdout != nil {
		stdout = *c.Stdout
	}
	return Options{
		Level:  c.Level,
		Stdout: stdout,
		File:   c.File,
	}
}
