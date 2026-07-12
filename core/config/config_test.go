package config

import (
	"os"
	"testing"

	"github.com/BullionBear/seq/core/tradingmode"
)

func TestLoadConfigFromBytes_ExpandsCatalogTokenPlaceholder(t *testing.T) {
	t.Setenv("CATALOG_API_TOKEN", "test-token-from-env")

	yaml := `
catalog:
  base_url: https://cpanel.example.test
  api_token: ${CATALOG_API_TOKEN}
logger:
  level: info
  output: stdout
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if cfg.Catalog.APIToken != "test-token-from-env" {
		t.Fatalf("APIToken = %q, want expanded env value", cfg.Catalog.APIToken)
	}
	if cfg.TradingMode != string(tradingmode.ModePaper) {
		t.Fatalf("TradingMode = %q, want default %q", cfg.TradingMode, tradingmode.ModePaper)
	}
}

func TestLoadConfigFromBytes_FillsEmptyTokenFromEnv(t *testing.T) {
	t.Setenv("CATALOG_API_TOKEN", "filled-from-env")

	yaml := `
catalog:
  base_url: https://cpanel.example.test
  api_token: ""
logger:
  level: info
  output: stdout
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if cfg.Catalog.APIToken != "filled-from-env" {
		t.Fatalf("APIToken = %q, want env fill-in", cfg.Catalog.APIToken)
	}
}

func TestLoadConfigFromBytes_MissingEnvLeavesEmpty(t *testing.T) {
	_ = os.Unsetenv("CATALOG_API_TOKEN_UNSET_TEST")
	// Ensure the preferred env is also unset for this case.
	t.Setenv("CATALOG_API_TOKEN", "")

	yaml := `
catalog:
  base_url: https://example.test
  api_token: ${CATALOG_API_TOKEN_UNSET_TEST}
logger:
  level: info
  output: stdout
`
	cfg, err := LoadConfigFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if cfg.Catalog.APIToken != "" {
		t.Fatalf("APIToken = %q, want empty when env unset", cfg.Catalog.APIToken)
	}
}
