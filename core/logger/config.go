package logger

// Config contains logger configuration.
// Logs always go to stdout. If Path is set, logs are also written to a file.
type Config struct {
	Level          string `yaml:"level"`
	Path           string `yaml:"path,omitempty"`    // If set, also write logs to this file
	MaxByteSize    int    `yaml:"max_byte_size"`     // Max size in bytes before rotation (0 = no rotation)
	MaxBackupFiles int    `yaml:"max_backup_files"`  // Max number of backup files to keep (0 = keep all)
}

// ToOptions converts a Config to an Options struct for Init().
func (c Config) ToOptions() Options {
	return Options{
		Level:          c.Level,
		Path:           c.Path,
		MaxByteSize:    c.MaxByteSize,
		MaxBackupFiles: c.MaxBackupFiles,
	}
}
