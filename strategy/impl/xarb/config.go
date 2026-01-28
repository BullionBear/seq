package xarb

type XArbConfig struct {
	QuotingSymbolName string `yaml:"quoting_symbol_name"`
	HedgingSymbolName string `yaml:"hedging_symbol_name"`
}
