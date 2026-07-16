package config

import (
	"os"
	"strings"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/core/telemetry"
	"github.com/BullionBear/seq/core/tradingmode"
	"github.com/BullionBear/seq/node"
	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	// TradingMode is paper|live. Empty defaults to paper after load.
	// Live also requires SEQ_ALLOW_LIVE at process start.
	TradingMode string                    `yaml:"trading_mode"`
	Logger      logger.Config             `yaml:"logger"`
	Catalog     catalog.Config            `yaml:"catalog"`
	MsgBus      msgbus.Config             `yaml:"msgbus"`
	ExecRouter  []adapter.ExecRouterEntry `yaml:"execrouter"`
	DataRouter  []adapter.DataRouterEntry `yaml:"datarouter"`
	Node        node.Config               `yaml:"node"`
	// Runtime fences the Go runtime (GC off + memory limit, GOMAXPROCS);
	// see docs/DEPLOYMENT.md.
	Runtime telemetry.RuntimeConfig `yaml:"runtime"`
	// Metrics enables the /metrics + /gc HTTP endpoints.
	Metrics telemetry.MetricsConfig `yaml:"metrics"`
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return LoadConfigFromBytes(data)
}

// LoadConfigFromBytes loads configuration from YAML bytes.
func LoadConfigFromBytes(data []byte) (*AppConfig, error) {
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	applyEnvSecrets(&cfg)
	return &cfg, nil
}

// LoadConfigFromString loads configuration from a YAML string.
func LoadConfigFromString(data string) (*AppConfig, error) {
	return LoadConfigFromBytes([]byte(data))
}

func (c *AppConfig) applyDefaults() {
	if strings.TrimSpace(c.TradingMode) == "" {
		c.TradingMode = string(tradingmode.ModePaper)
	}
}

// applyEnvSecrets expands ${VAR} placeholders in secret fields and fills
// empty catalog.api_token from CATALOG_API_TOKEN when set.
func applyEnvSecrets(cfg *AppConfig) {
	cfg.Catalog.APIToken = os.ExpandEnv(cfg.Catalog.APIToken)
	if cfg.Catalog.APIToken == "" {
		if tok := os.Getenv("CATALOG_API_TOKEN"); tok != "" {
			cfg.Catalog.APIToken = tok
		}
	}
}
