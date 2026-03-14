package xarb

// XArbConfig contains strategy-specific configuration for the XArb strategy
type XArbConfig struct {
	QuotingSymbolUniversalTicker string `yaml:"quoting_symbol_universal_ticker"`
	HedgingSymbolUniversalTicker string `yaml:"hedging_symbol_universal_ticker"`

	// Wallet configuration for trading
	// QuotingWallet is the wallet used for placing quotes (making markets)
	QuotingWallet string `yaml:"quoting_wallet,omitempty"`
	// HedgingWallet is the wallet used for hedging trades
	HedgingWallet string `yaml:"hedging_wallet,omitempty"`

	// Algo parameters
	Side              string  `yaml:"side"`
	ProfitBps         float64 `yaml:"profit_bps"`
	Qty               float64 `yaml:"qty"`
	PriceToleranceBps float64 `yaml:"price_tolerance_bps"`
	OrderTTL          string  `yaml:"order_ttl,omitempty"`
}
