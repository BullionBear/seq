package logger

// Config contains logger configuration.
// This is the YAML-mapped config type; it maps directly to Options for Init().
type Config struct {
	Level          string `yaml:"level"`
	Output         string `yaml:"output"`           // "stdout" or "file"
	Path           string `yaml:"path"`             // Required when output is "file"
	MaxByteSize    int    `yaml:"max_byte_size"`    // Max size in bytes before rotation (0 = no rotation)
	MaxBackupFiles int    `yaml:"max_backup_files"` // Max number of backup files to keep (0 = keep all)
}

// ToOptions converts a Config to an Options struct for Init().
func (c Config) ToOptions() Options {
	return Options{
		Level:          c.Level,
		Output:         c.Output,
		Path:           c.Path,
		MaxByteSize:    c.MaxByteSize,
		MaxBackupFiles: c.MaxBackupFiles,
	}
}
