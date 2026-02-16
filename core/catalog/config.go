package catalog

// Config contains catalog service configuration.
type Config struct {
	BaseURL  string `yaml:"base_url"`
	APIToken string `yaml:"api_token"`
}
