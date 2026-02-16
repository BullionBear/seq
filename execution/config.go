package execution

// Config contains execution engine configuration.
type Config struct {
	Accounts []AccountConfig `yaml:"accounts"`
}

// AccountConfig contains configuration for a single execution account.
type AccountConfig struct {
	ID      int    `yaml:"id"`      // Account ID (optional, can be resolved from name)
	Account string `yaml:"account"` // Account name to look up in catalog
}
