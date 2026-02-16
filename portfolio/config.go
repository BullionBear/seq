package portfolio

// Config contains portfolio engine configuration.
type Config struct {
	Accounts []AccountConfig `yaml:"accounts"`
}

// AccountConfig contains configuration for a single portfolio account.
type AccountConfig struct {
	ID      int    `yaml:"id"`      // Account ID (optional, can be resolved from name)
	Account string `yaml:"account"` // Account name to look up in catalog
}
