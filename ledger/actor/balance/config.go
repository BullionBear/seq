package balance

// BalanceConfig contains configuration for a single balance actor instance.
// Each balance actor manages exactly one wallet.
type BalanceConfig struct {
	Wallet string `yaml:"wallet"` // Wallet name to track (e.g. "bn-hephe-spot")
}
