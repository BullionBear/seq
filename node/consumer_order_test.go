package node

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/BullionBear/seq/adapter"
	"github.com/BullionBear/seq/core/catalog"
	"gopkg.in/yaml.v3"

	// Register actor factories via init() — same set as cmd/main.go for xarb.
	_ "github.com/BullionBear/seq/data/actor/orderbook"
	_ "github.com/BullionBear/seq/execution/actor/oms"
	_ "github.com/BullionBear/seq/ledger/actor/inventory"
	_ "github.com/BullionBear/seq/risk/actor/ratelimiter"
	_ "github.com/BullionBear/seq/strategy/actor/xarb"
)

// wantConsumerOrder is a correctness contract, not an implementation detail.
//
// If this changes because an actor was ADDED: confirm the new actor sits in the
// correct phase, then update this list.
// If this changes because node.initEngines was REORDERED: that is the bug. Do
// not update this list.
//
// See docs/CONSUMER_ORDER.md.
var wantConsumerOrder = []string{
	"orderbook-binance",       // data      / ingest
	"orderbook-bybit",         // data      / ingest
	"oms",                     // execution / order
	"inventory-bybit-unified", // ledger    / account
	"inventory-bn-spot",       // ledger    / account
	// rate-limiter-bybit: risk guard with nil topics — not registered (PR #33)
	"xarb-uni-buy-5bps",  // strategy / decide
	"xarb-uni-sell-5bps", // strategy / decide
}

// fixtureConfig is the subset of AppConfig needed to drive initEngines
// headless. Decoded here (not via core/config) to avoid an import cycle:
// core/config imports node.
type fixtureConfig struct {
	Catalog    catalog.Config            `yaml:"catalog"`
	DataRouter []adapter.DataRouterEntry `yaml:"datarouter"`
	Node       Config                    `yaml:"node"`
}

func loadTestConfig(t *testing.T, path string) fixtureConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config %s: %v", path, err)
	}
	var cfg fixtureConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config %s: %v", path, err)
	}
	if cfg.Catalog.Instruments != "" && !filepath.IsAbs(cfg.Catalog.Instruments) {
		cfg.Catalog.Instruments = filepath.Join(filepath.Dir(path), cfg.Catalog.Instruments)
	}
	return cfg
}

func TestNode_ConsumerOrder(t *testing.T) {
	cfg := loadTestConfig(t, "../config/xarb.yml")

	cat, err := catalog.NewCatalog(cfg.Catalog)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	n := NewNode(cat)
	// initEngines deliberately excludes setupExecutionClients so this runs
	// without venue credentials.
	if err := n.initEngines(cfg.Node, nil); err != nil {
		t.Fatalf("initEngines: %v", err)
	}

	got := n.msgBus.ConsumerNames()
	if !slices.Equal(got, wantConsumerOrder) {
		t.Fatalf("consumer order changed\n got: %v\nwant: %v\n"+
			"Dispatch order is a correctness contract — see docs/CONSUMER_ORDER.md",
			got, wantConsumerOrder)
	}
}

func TestNode_ConsumerPhasesNonDecreasing(t *testing.T) {
	cfg := loadTestConfig(t, "../config/xarb.yml")

	cat, err := catalog.NewCatalog(cfg.Catalog)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	n := NewNode(cat)
	if err := n.initEngines(cfg.Node, nil); err != nil {
		t.Fatalf("initEngines: %v", err)
	}
	if err := n.msgBus.AssertOrder(); err != nil {
		t.Fatalf("AssertOrder: %v", err)
	}
}
