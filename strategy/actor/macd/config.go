package macd

// Config is the YAML configuration for the MACD strategy.
type Config struct {
	UniversalTicker string  `yaml:"universal_ticker"`
	Wallet          string  `yaml:"wallet"`
	FastPeriod      int     `yaml:"fast_period"`
	SlowPeriod      int     `yaml:"slow_period"`
	SignalPeriod    int     `yaml:"signal_period"`
	ExecQty         float64 `yaml:"exec_qty"`
}
