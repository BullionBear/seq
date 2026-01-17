package strategy

import (
	"os"

	"gopkg.in/yaml.v3"
)

type StrategyConfig struct {
	Logger  ConfigLogger  `yaml:"logger"`
	Catalog ConfigCatalog `yaml:"catalog"`
	EMS     ConfigEMS     `yaml:"ems"`
	PMS     ConfigPMS     `yaml:"pms"`
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
