package balance

// BalanceConfig contains configuration for a single balance actor instance.
type BalanceConfig struct {
	ID      int    `yaml:"id"`      // Account ID
	Account string `yaml:"account"` // Account name in catalog
	Wallet  string `yaml:"wallet"`  // Wallet name for this account
	API     string `yaml:"api"`     // API key name for this account
}
