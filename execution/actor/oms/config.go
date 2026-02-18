package oms

// OMSConfig contains configuration for a single OMS actor instance.
type OMSConfig struct {
	ID      int    `yaml:"id"`      // Account ID
	Account string `yaml:"account"` // Account name in catalog
}
