package node

import (
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/portfolio"
	"github.com/BullionBear/seq/risk"
	"github.com/BullionBear/seq/strategy"
)

// Config contains node-level configuration (engines only).
type Config struct {
	Engine EngineConfig `yaml:"engine"`
}

// EngineConfig composes all engine configurations.
type EngineConfig struct {
	Data      data.Config      `yaml:"data"`
	Execution execution.Config `yaml:"execution"`
	Portfolio portfolio.Config `yaml:"portfolio"`
	Risk      risk.Config      `yaml:"risk"`
	Strategy  strategy.Config  `yaml:"strategy"`
}
