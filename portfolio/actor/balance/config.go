package balance

// BalanceConfig contains configuration for a single balance actor instance.
type BalanceConfig struct {
	Subscription []string `yaml:"subscription"` // Event topic names to subscribe to
}
