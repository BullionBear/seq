package config

import (
	"os"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/node"
	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	Logger  logger.Config  `yaml:"logger"`
	Catalog catalog.Config `yaml:"catalog"`
	MsgBus  msgbus.Config  `yaml:"msgbus"`
	Node    node.Config    `yaml:"node"`
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadConfigFromBytes loads configuration from YAML bytes.
func LoadConfigFromBytes(data []byte) (*AppConfig, error) {
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadConfigFromString loads configuration from a YAML string.
func LoadConfigFromString(data string) (*AppConfig, error) {
	return LoadConfigFromBytes([]byte(data))
}
