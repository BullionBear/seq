package xarb

type XArbConfig struct {
	QuotingSymbolID int `yaml:"quoting_symbol_id"`
	HedgingSymbolID int `yaml:"hedging_symbol_id"`
}
