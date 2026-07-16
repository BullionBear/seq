package node

import (
	"github.com/BullionBear/seq/data"
	"github.com/BullionBear/seq/execution"
	"github.com/BullionBear/seq/portfolio"
	"github.com/BullionBear/seq/risk"
	"github.com/BullionBear/seq/strategy"
)

// Config contains node-level configuration.
type Config struct {
	Engine   EngineConfig   `yaml:"engine"`
	Dispatch DispatchConfig `yaml:"dispatch"`
}

// DispatchConfig controls the dispatch-loop goroutine (P2-4). The loop is
// always pinned to one OS thread (runtime.LockOSThread); this selects what
// it does when idle.
type DispatchConfig struct {
	// IdleStrategy is "gosched" (default) or "spin".
	//   gosched: yield to the Go scheduler whenever the loop finds no work.
	//     Cooperative; correct on shared cores.
	//   spin: busy-spin SpinBudget idle iterations before yielding once.
	//     Minimizes wake-up latency; requires a dedicated core
	//     (see docs/DEPLOYMENT.md).
	IdleStrategy string `yaml:"idle_strategy"`
	// SpinBudget is the number of consecutive idle iterations a spin-strategy
	// loop performs before it yields to the scheduler. 0 selects
	// DefaultSpinBudget. Ignored for the gosched strategy.
	SpinBudget int `yaml:"spin_budget"`
}

const (
	// IdleStrategyGosched yields to the Go scheduler on every idle iteration.
	IdleStrategyGosched = "gosched"
	// IdleStrategySpin busy-spins a bounded number of idle iterations
	// before yielding.
	IdleStrategySpin = "spin"

	// DefaultSpinBudget bounds a spin-strategy busy loop. At ~ns-scale ring
	// probes this is a few microseconds of spinning per yield.
	DefaultSpinBudget = 4096
)

// EngineConfig composes all engine configurations.
type EngineConfig struct {
	Data      data.Config      `yaml:"data"`
	Execution execution.Config `yaml:"execution"`
	Portfolio portfolio.Config `yaml:"portfolio"`
	Risk      risk.Config      `yaml:"risk"`
	Strategy  strategy.Config  `yaml:"strategy"`
}
