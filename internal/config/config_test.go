package config

import (
	"os"
	"path/filepath"
	"testing"
)

const configYAML = `
logger:
  level: debug
  path: /tmp/test/seq.log
ems:
  url: http://localhost:8080
pms:
  url: http://localhost:8081
`

func TestLoadConfigFromString(t *testing.T) {
	config, err := LoadConfigFromString(configYAML)
	if err != nil {
		t.Fatalf("LoadConfigFromString failed: %v", err)
	}
	if config == nil {
		t.Fatalf("Config is nil")
	}

	// Test logger config
	if config.Logger.Level != "debug" {
		t.Errorf("Expected logger level 'debug', got '%s'", config.Logger.Level)
	}
	if config.Logger.Path != "/tmp/test/seq.log" {
		t.Errorf("Expected logger path '/tmp/test/seq.log', got '%s'", config.Logger.Path)
	}

	// Test EMS config
	if config.EMS.URL != "http://localhost:8080" {
		t.Errorf("Expected EMS URL 'http://localhost:8080', got '%s'", config.EMS.URL)
	}

	// Test PMS config
	if config.PMS.URL != "http://localhost:8081" {
		t.Errorf("Expected PMS URL 'http://localhost:8081', got '%s'", config.PMS.URL)
	}
}

func TestLoadConfigFromBytes(t *testing.T) {
	data := []byte(configYAML)
	config, err := LoadConfigFromBytes(data)
	if err != nil {
		t.Fatalf("LoadConfigFromBytes failed: %v", err)
	}
	if config == nil {
		t.Fatalf("Config is nil")
	}
	if config.Logger.Level != "debug" {
		t.Errorf("Expected logger level 'debug', got '%s'", config.Logger.Level)
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config from file
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if config == nil {
		t.Fatalf("Config is nil")
	}

	// Verify loaded config
	if config.Logger.Level != "debug" {
		t.Errorf("Expected logger level 'debug', got '%s'", config.Logger.Level)
	}
	if config.Logger.Path != "/tmp/test/seq.log" {
		t.Errorf("Expected logger path '/tmp/test/seq.log', got '%s'", config.Logger.Path)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}
