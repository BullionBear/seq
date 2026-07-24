package config

import (
	"strings"
	"testing"

	"github.com/BullionBear/seq/core/tradingmode"
)

func TestLoadConfigFromBytes_DefaultTradingModeIsPaper(t *testing.T) {
	yaml := `
catalog:
  instruments: ./config/instruments.json
logger:
  level: info
  stdout: true
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if cfg.TradingMode != string(tradingmode.ModePaper) {
		t.Fatalf("TradingMode = %q, want %q", cfg.TradingMode, tradingmode.ModePaper)
	}
}

func TestLoadConfigFromBytes_ExpandsAPIKeySecrets(t *testing.T) {
	t.Setenv("TEST_API_KEY", "key-from-env")
	t.Setenv("TEST_API_SECRET", "secret-from-env")

	yaml := `
catalog:
  instruments: ./config/instruments.json
  accounts:
    - name: test-account
      exchange: Bybit
      api_keys:
        - name: test-hmac
          type: HMAC
          key: ${TEST_API_KEY}
          secret: ${TEST_API_SECRET}
logger:
  level: info
  stdout: true
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	apiKey := cfg.Catalog.Accounts[0].APIKeys[0]
	if apiKey.Key != "key-from-env" {
		t.Fatalf("Key = %q, want expanded env value", apiKey.Key)
	}
	if apiKey.Secret != "secret-from-env" {
		t.Fatalf("Secret = %q, want expanded env value", apiKey.Secret)
	}
}

func TestLoadConfigFromBytes_MissingEnvLeavesEmpty(t *testing.T) {
	t.Setenv("TEST_API_KEY_UNSET", "")

	yaml := `
catalog:
  instruments: ./config/instruments.json
  accounts:
    - name: test-account
      exchange: Binance
      api_keys:
        - name: test-hmac
          type: HMAC
          key: ${TEST_API_KEY_UNSET}
          secret: ""
logger:
  level: info
  stdout: true
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if got := cfg.Catalog.Accounts[0].APIKeys[0].Key; got != "" {
		t.Fatalf("Key = %q, want empty when env unset", got)
	}
}

func TestLoadConfigFromBytes_RejectsUnknownFields(t *testing.T) {
	yaml := `
catalog:
  instruments: ./config/instruments.json
logger:
  level: info
  stdout: true
node:
  engine:
    risk:
      actor: []
      checker:
        - type: ratelimit
`
	_, err := LoadConfigFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for removed risk.checker field")
	}
	if !strings.Contains(err.Error(), "checker") {
		t.Fatalf("error %v should mention checker", err)
	}
}
