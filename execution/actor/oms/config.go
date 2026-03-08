package oms

// OMSConfig contains configuration for a single OMS actor instance.
type OMSConfig struct {
	Subscription []string `yaml:"subscription"` // Event topic names to subscribe to
}
