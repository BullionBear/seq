package catalog

// Config contains catalog service configuration.
type Config struct {
	// Instruments is the path to a JSON file containing the full list of
	// tradable symbols (see Symbol for the expected shape).
	Instruments string `yaml:"instruments"`
	// Accounts defines exchange accounts, their API credentials and wallets.
	Accounts []AccountConfig `yaml:"accounts"`
}

// AccountConfig defines an exchange account in the YAML config.
type AccountConfig struct {
	Name     string          `yaml:"name"`
	Exchange string          `yaml:"exchange"` // Binance | Bybit
	APIKeys  []APIKeyConfig  `yaml:"api_keys"`
	Wallets  []WalletConfig  `yaml:"wallets"`
}

// APIKeyConfig defines API credentials in the YAML config.
// Key, Secret and Passphrase support ${ENV_VAR} expansion.
type APIKeyConfig struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"` // HMAC | RSA | ED25519
	Key        string `yaml:"key"`
	Secret     string `yaml:"secret"`
	Passphrase string `yaml:"passphrase"`
}

// WalletConfig defines a wallet in the YAML config.
type WalletConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // spot | umargin | cmargin | leverage | unified
}
