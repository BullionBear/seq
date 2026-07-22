package rotate

// SyncPolicy controls how aggressively writes are flushed to durable storage.
type SyncPolicy string

const (
	// SyncNone relies on write(2) into the kernel page cache only.
	// Survives process crash; does not survive power loss.
	SyncNone SyncPolicy = "none"
	// SyncRotate fsyncs at each rotation boundary (default).
	SyncRotate SyncPolicy = "rotate"
	// SyncPeriodic fsyncs on an external cadence (e.g. observer 1s tick).
	SyncPeriodic SyncPolicy = "periodic"
	// SyncEach fsyncs after every Write. Strongest and most expensive.
	SyncEach SyncPolicy = "each"
)

// Policy describes runtime rotation rules for one output file.
// Date and size are OR conditions: either trigger causes a rollover.
type Policy struct {
	Dir        string     // output directory
	BaseName   string     // filename prefix, e.g. "seq" / "msg"
	Ext        string     // extension without dot, e.g. "log" / "jsonl"
	MaxBytes   int64      // size trigger; <=0 disables
	Daily      bool       // UTC day-boundary trigger
	MaxBackups int        // max retained files (0 = unlimited)
	MaxAgeDays int        // max age in days (0 = no age-based deletion)
	Sync       SyncPolicy // durability policy; empty treated as SyncRotate
}

// FileConfig is the shared YAML block for logger and msglog file output.
type FileConfig struct {
	Dir        string     `yaml:"dir"`
	Name       string     `yaml:"name"`
	MaxBytes   int64      `yaml:"max_bytes"`
	Daily      bool       `yaml:"daily"`
	MaxBackups int        `yaml:"max_backups"`
	MaxAgeDays int        `yaml:"max_age_days"`
	Sync       SyncPolicy `yaml:"sync"`
}

// ToPolicy converts FileConfig into a runtime Policy. ext is fixed by the caller
// ("log" or "jsonl") and is not configurable.
func (c FileConfig) ToPolicy(ext string) Policy {
	sync := c.Sync
	if sync == "" {
		sync = SyncRotate
	}
	return Policy{
		Dir:        c.Dir,
		BaseName:   c.Name,
		Ext:        ext,
		MaxBytes:   c.MaxBytes,
		Daily:      c.Daily,
		MaxBackups: c.MaxBackups,
		MaxAgeDays: c.MaxAgeDays,
		Sync:       sync,
	}
}

// Enabled reports whether file output is configured (non-empty directory).
func (c FileConfig) Enabled() bool {
	return c.Dir != ""
}
