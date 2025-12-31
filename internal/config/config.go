package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Logger ConfigLogger `yaml:"logger"`
	EMS    ConfigEMS    `yaml:"ems"`
	PMS    ConfigPMS    `yaml:"pms"`
}

// ConfigLogger contains logger configuration
type ConfigLogger struct {
	Level string `yaml:"level"`
	Path  string `yaml:"path"`
}

// ConfigEMS contains EMS (Event Management System) configuration
type ConfigEMS struct {
	URL string `yaml:"url"`
}

// ConfigPMS contains PMS (Portfolio Management System) configuration
type ConfigPMS struct {
	URL string `yaml:"url"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfigFromBytes loads configuration from YAML bytes
func LoadConfigFromBytes(data []byte) (*Config, error) {
	var config Config
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// LoadConfigFromString loads configuration from a YAML string
func LoadConfigFromString(data string) (*Config, error) {
	return LoadConfigFromBytes([]byte(data))
}
