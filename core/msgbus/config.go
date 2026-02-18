package msgbus

// Config contains msgbus configuration.
type Config struct {
	MsgLog MsgLogConfig `yaml:"msglog"`
}

// MsgLogConfig contains binary event/command .dat file logging configuration.
type MsgLogConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable binary event/command logging
	Dir     string `yaml:"dir"`     // Directory for .dat files
}
