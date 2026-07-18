// Package tradingmode defines the process-wide paper vs live execution gate.
//
// Venue order mutations follow trading_mode (or SEQ_TRADING_MODE). The default
// mode is always paper.
package tradingmode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Mode is the runtime trading posture for Seq.
type Mode string

const (
	// ModePaper is the default. Venue order submit/cancel paths must refuse
	// live mutations.
	ModePaper Mode = "paper"

	// ModeLive allows venue order mutations.
	ModeLive Mode = "live"
)

// EnvTradingMode optionally overrides the YAML trading_mode value.
const EnvTradingMode = "SEQ_TRADING_MODE"

var (
	// ErrPaperMode is returned by execution paths that refuse live venue
	// order mutations while the process is in paper mode.
	ErrPaperMode = errors.New("trading mode is paper: live venue order mutation refused")

	mu      sync.RWMutex
	current = ModePaper
)

// Parse converts a config/env string into a Mode. Empty string means paper.
func Parse(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ModePaper):
		return ModePaper, nil
	case string(ModeLive):
		return ModeLive, nil
	default:
		return "", fmt.Errorf("invalid trading_mode %q: want paper or live", s)
	}
}

// Resolve picks the effective trading mode from config and environment.
//
// Precedence:
//  1. SEQ_TRADING_MODE if set
//  2. configValue (empty → paper)
func Resolve(configValue string, getenv func(string) string) (Mode, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	raw := strings.TrimSpace(configValue)
	if envMode := strings.TrimSpace(getenv(EnvTradingMode)); envMode != "" {
		raw = envMode
	}

	return Parse(raw)
}

// Set installs the process-wide trading mode. Call once at boot after Resolve.
func Set(mode Mode) {
	if mode == "" {
		mode = ModePaper
	}
	mu.Lock()
	current = mode
	mu.Unlock()
}

// Current returns the process-wide trading mode (defaults to paper).
func Current() Mode {
	mu.RLock()
	defer mu.RUnlock()
	if current == "" {
		return ModePaper
	}
	return current
}

// RequireLive returns ErrPaperMode unless the process is in live mode.
func RequireLive() error {
	if !Current().IsLive() {
		return ErrPaperMode
	}
	return nil
}

// ResetForTest restores the default paper mode. Tests only.
func ResetForTest() {
	Set(ModePaper)
}

// IsLive reports whether mode permits live venue order mutations.
func (m Mode) IsLive() bool {
	return m == ModeLive
}

// String returns the canonical mode name.
func (m Mode) String() string {
	if m == "" {
		return string(ModePaper)
	}
	return string(m)
}
