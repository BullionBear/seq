package msgbus

// Config contains msgbus configuration.
type Config struct {
	MsgLog MsgLogConfig `yaml:"msglog"`
}

// MsgLogConfig contains plaintext event/command JSONL logging configuration.
type MsgLogConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable plaintext event/command JSONL logging
	Dir     string `yaml:"dir"`     // Directory for .jsonl files
}
