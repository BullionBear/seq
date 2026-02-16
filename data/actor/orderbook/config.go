package orderbook

// OrderbookConfig contains configuration for a single orderbook actor instance.
type OrderbookConfig struct {
	Symbol string `yaml:"symbol"` // Universal ticker of the symbol to track
}
