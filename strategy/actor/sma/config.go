package sma

// Config is the YAML configuration for the SMA strategy.
type Config struct {
	UniversalTicker string `yaml:"universal_ticker"`
	Interval        string `yaml:"interval"` // e.g. 1m, 5m, 1h
	Period          int    `yaml:"period"`   // window length n
}
