package xarb

// XArbConfig contains strategy-specific configuration for the XArb strategy
type XArbConfig struct {
	QuotingSymbolUniversalTicker string `yaml:"quoting_symbol_universal_ticker"`
	HedgingSymbolUniversalTicker string `yaml:"hedging_symbol_universal_ticker"`

	// Account configuration for trading
	// QuotingAccount is used for placing quotes (making markets)
	QuotingAccount string `yaml:"quoting_account,omitempty"`
	// HedgingAccount is used for hedging trades
	HedgingAccount string `yaml:"hedging_account,omitempty"`

	// Algo parameters
	Side              string  `yaml:"side"`
	ProfitBps         float64 `yaml:"profit_bps"`
	Qty               float64 `yaml:"qty"`
	PriceToleranceBps float64 `yaml:"price_tolerance_bps"`
}
