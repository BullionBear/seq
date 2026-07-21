package inventory

// InventoryConfig contains configuration for a single inventory actor instance.
// Each inventory actor manages exactly one wallet.
type InventoryConfig struct {
	Wallet string `yaml:"wallet"` // Wallet name to track (e.g. "bn-hephe-spot")
}
