// Package tradingmode defines the process-wide paper vs live execution gate.
//
// Live venue order mutations require an explicit dual opt-in: config
// trading_mode=live (or SEQ_TRADING_MODE=live) and environment
// SEQ_ALLOW_LIVE=1|true|yes. The default mode is always paper. Enabling live
// trading still requires separate CEO/board approval outside this package.
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

	// ModeLive allows venue order mutations when also permitted by env.
	ModeLive Mode = "live"
)

// EnvAllowLive is the hard-to-miss environment gate for live mode.
// Accepted truthy values: 1, true, yes (case-insensitive).
const EnvAllowLive = "SEQ_ALLOW_LIVE"

// EnvTradingMode optionally overrides the YAML trading_mode value.
// When set to live, SEQ_ALLOW_LIVE is still required.
const EnvTradingMode = "SEQ_TRADING_MODE"

var (
	// ErrLiveNotAllowed is returned when live mode is requested without
	// SEQ_ALLOW_LIVE set to a truthy value.
	ErrLiveNotAllowed = errors.New("live trading mode refused: set SEQ_ALLOW_LIVE=1 (or true/yes) after CEO/board approval; defaulting to paper is mandatory without this gate")

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

// AllowLiveEnv reports whether getenv(SEQ_ALLOW_LIVE) is an explicit truthy opt-in.
func AllowLiveEnv(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(getenv(EnvAllowLive))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// Resolve picks the effective trading mode from config and environment.
//
// Precedence:
//  1. SEQ_TRADING_MODE if set (must still pass SEQ_ALLOW_LIVE when live)
//  2. configValue (empty → paper)
//
// Live always requires SEQ_ALLOW_LIVE; otherwise Resolve returns ErrLiveNotAllowed.
func Resolve(configValue string, getenv func(string) string) (Mode, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	raw := strings.TrimSpace(configValue)
	if envMode := strings.TrimSpace(getenv(EnvTradingMode)); envMode != "" {
		raw = envMode
	}

	mode, err := Parse(raw)
	if err != nil {
		return "", err
	}
	if mode == ModeLive && !AllowLiveEnv(getenv) {
		return "", ErrLiveNotAllowed
	}
	return mode, nil
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
