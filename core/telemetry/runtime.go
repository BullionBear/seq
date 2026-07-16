// Package telemetry provides runtime fencing (P2-4) and the metrics hook
// exposing runtime/metrics histograms plus the msgbus overflow counters.
package telemetry

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
)

// RuntimeConfig fences the Go runtime for latency-critical deployments
// (P2-4). The supported production configuration after the Phase 2
// allocation work is gc_off=true with a memory limit: the GC becomes a
// memory fuse rather than a periodic event, because the hot path allocates
// nothing in steady state.
//
// The equivalent environment variables (GOGC=off, GOMEMLIMIT, GOMAXPROCS)
// also work without any of these fields; config wins when both are set
// because Apply runs after the runtime reads the environment.
type RuntimeConfig struct {
	// GCOff disables periodic garbage collection (GOGC=off). Refused unless
	// a memory limit is in force (config or GOMEMLIMIT), which keeps the GC
	// available as a fuse against leaks.
	GCOff bool `yaml:"gc_off"`
	// MemLimitBytes sets the Go soft memory limit (GOMEMLIMIT).
	// 0 leaves the current limit untouched.
	MemLimitBytes int64 `yaml:"mem_limit_bytes"`
	// GOMAXPROCS caps the number of Ps. 0 leaves the default.
	// See docs/DEPLOYMENT.md for sizing guidance.
	GOMAXPROCS int `yaml:"gomaxprocs"`
}

// Apply installs the runtime fencing. Call once at startup, before engines
// start.
func (c RuntimeConfig) Apply() error {
	if c.GOMAXPROCS > 0 {
		runtime.GOMAXPROCS(c.GOMAXPROCS)
	}
	if c.MemLimitBytes > 0 {
		debug.SetMemoryLimit(c.MemLimitBytes)
	}
	if c.GCOff {
		// SetMemoryLimit(-1) reads the current limit without changing it;
		// MaxInt64 means "no limit set" (neither config nor GOMEMLIMIT).
		if debug.SetMemoryLimit(-1) == math.MaxInt64 {
			return fmt.Errorf("telemetry: gc_off requires a memory limit (mem_limit_bytes or GOMEMLIMIT); refusing to disable GC without a fuse")
		}
		debug.SetGCPercent(-1)
	}
	return nil
}

// ForceGC runs a full garbage collection cycle. It backs the /gc endpoint of
// the metrics server: with gc_off, operators can trigger collections during
// declared quiet windows instead of relying on the memory-limit fuse.
func ForceGC() {
	runtime.GC()
}
