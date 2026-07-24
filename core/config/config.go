package config

import (
	"bytes"
	"os"
	"path/filepath"
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

// LoadConfig loads configuration from a YAML file. A relative
// catalog.instruments path is resolved against the config file's directory.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		return nil, err
	}
	if cfg.Catalog.Instruments != "" && !filepath.IsAbs(cfg.Catalog.Instruments) {
		cfg.Catalog.Instruments = filepath.Join(filepath.Dir(path), cfg.Catalog.Instruments)
	}
	return cfg, nil
}

// LoadConfigFromBytes loads configuration from YAML bytes.
// Unknown fields are rejected (KnownFields) so stale keys like the removed
// risk.checker block fail closed at load time.
func LoadConfigFromBytes(data []byte) (*AppConfig, error) {
	var cfg AppConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
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

// applyEnvSecrets expands ${VAR} placeholders in secret fields of the
// catalog account API keys.
func applyEnvSecrets(cfg *AppConfig) {
	for i := range cfg.Catalog.Accounts {
		for j := range cfg.Catalog.Accounts[i].APIKeys {
			key := &cfg.Catalog.Accounts[i].APIKeys[j]
			key.Key = os.ExpandEnv(key.Key)
			key.Secret = os.ExpandEnv(key.Secret)
			key.Passphrase = os.ExpandEnv(key.Passphrase)
		}
	}
}
