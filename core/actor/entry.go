package actor

// Entry is the universal config entry for an actor across all engines.
// Every engine config has an Actor []Entry field.
type Entry struct {
	Type   string         `yaml:"type"`
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
}
