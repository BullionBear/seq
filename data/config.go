package data

// Config contains data engine configuration.
type Config struct {
	Subscriptions []SubscriptionConfig `yaml:"subscriptions"`
}

// SubscriptionConfig is per-symbol data subscription config.
type SubscriptionConfig struct {
	Symbol   string       `yaml:"symbol"`             // Universal ticker (required)
	Endpoint string       `yaml:"endpoint,omitempty"` // Regional endpoint: bybit, bybit_tr, bybit_eu
	Depth    *DepthConfig `yaml:"depth,omitempty"`    // Depth subscription options
	Trade    *TradeConfig `yaml:"trade,omitempty"`    // Trade subscription options
}

// DepthConfig configures depth stream subscription.
type DepthConfig struct {
	Type     string `yaml:"type,omitempty"`      // delta, snapshot, depth5, depth10, depth20 (binance)
	PushRate string `yaml:"push_rate,omitempty"` // 100ms, 1000ms (binance)
	Levels   int    `yaml:"levels,omitempty"`    // 1, 50, 200, 500 (bybit)
}

// TradeConfig configures trade stream subscription.
type TradeConfig struct {
	Type string `yaml:"type,omitempty"` // trade, aggTrade
}
