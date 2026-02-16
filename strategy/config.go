package strategy

import (
	"os"

	"gopkg.in/yaml.v3"
)

type StrategyConfig struct {
	Logger    ConfigLogger    `yaml:"logger"`
	Catalog   ConfigCatalog   `yaml:"catalog"`
	Data      []ConfigDataSub `yaml:"data"`      // Data subscriptions (flat list, per-symbol)
	Execution ConfigExecution `yaml:"execution"` // Execution accounts
	MsgLog    ConfigMsgLog    `yaml:"msglog"`    // Binary event/command logging
	Strategy  map[string]any  `yaml:"strategy"`
}

// ConfigDataSub is per-symbol data subscription config
type ConfigDataSub struct {
	Symbol   string       `yaml:"symbol"`             // Universal ticker (required)
	Endpoint string       `yaml:"endpoint,omitempty"` // Regional endpoint: bybit, bybit_tr, bybit_eu
	Depth    *ConfigDepth `yaml:"depth,omitempty"`    // Depth subscription options
	Trade    *ConfigTrade `yaml:"trade,omitempty"`    // Trade subscription options
}

// ConfigDepth configures depth stream subscription
type ConfigDepth struct {
	Type     string `yaml:"type,omitempty"`      // delta, snapshot, depth5, depth10, depth20 (binance)
	PushRate string `yaml:"push_rate,omitempty"` // 100ms, 1000ms (binance)
	Levels   int    `yaml:"levels,omitempty"`    // 1, 50, 200, 500 (bybit)
}

// ConfigTrade configures trade stream subscription
type ConfigTrade struct {
	Type string `yaml:"type,omitempty"` // trade, aggTrade
}

// ConfigExecution is a list of execution account configs
type ConfigExecution []ConfigExecutionAccount

// ConfigExecutionAccount contains configuration for a single execution account
type ConfigExecutionAccount struct {
	ID      int    `yaml:"id"`      // Account ID (optional, can be resolved from name)
	Account string `yaml:"account"` // Account name to look up in catalog
}

// ConfigCatalog contains catalog configuration
type ConfigCatalog struct {
	BaseURL  string `yaml:"base_url"`
	APIToken string `yaml:"api_token"`
}

// ConfigLogger contains logger configuration
type ConfigLogger struct {
	Level          string `yaml:"level"`
	Output         string `yaml:"output"`           // "stdout" or "file"
	Path           string `yaml:"path"`             // Required when output is "file"
	MaxByteSize    int    `yaml:"max_byte_size"`    // Max size in bytes before rotation (0 = no rotation)
	MaxBackupFiles int    `yaml:"max_backup_files"` // Max number of backup files to keep (0 = keep all)
}

// ConfigMsgLog contains binary event/command .dat file logging configuration
type ConfigMsgLog struct {
	Enabled bool   `yaml:"enabled"` // Enable binary event/command logging
	Dir     string `yaml:"dir"`     // Directory for .dat files
}

// ConfigEMS contains EMS (Event Management System) configuration
type ConfigEMS struct {
	URL string `yaml:"url"`
}

// ConfigPMS contains PMS (Portfolio Management System) configuration
type ConfigPMS struct {
	URL      string         `yaml:"url"`
	Database ConfigDatabase `yaml:"database"`
}

// ConfigDatabase contains database configuration
type ConfigDatabase struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"` // disable, allow, prefer, require, verify-ca, verify-full
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*StrategyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config StrategyConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfigFromBytes loads configuration from YAML bytes
func LoadConfigFromBytes(data []byte) (*StrategyConfig, error) {
	var config StrategyConfig
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// LoadConfigFromString loads configuration from a YAML string
func LoadConfigFromString(data string) (*StrategyConfig, error) {
	return LoadConfigFromBytes([]byte(data))
}
