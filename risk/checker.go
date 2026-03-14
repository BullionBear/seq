package risk

import (
	"fmt"

	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/risk/rule"
)

// Checker aggregates risk rules and evaluates them sequentially.
// The first rule that returns an error causes the check to fail.
type Checker struct {
	rules []rule.Rule
}

// Check runs all rules against the given command.
// Returns nil if all rules pass, or the first error encountered.
func (c *Checker) Check(cmd command.RiskCheck) error {
	for _, r := range c.rules {
		if err := r.Check(cmd); err != nil {
			return err
		}
	}
	return nil
}

// CheckerBuilder constructs a Checker by accumulating rules.
type CheckerBuilder struct {
	rules []rule.Rule
}

// NewCheckerBuilder creates a new CheckerBuilder.
func NewCheckerBuilder() *CheckerBuilder {
	return &CheckerBuilder{}
}

// AddRule appends a rule to the builder.
func (b *CheckerBuilder) AddRule(r rule.Rule) *CheckerBuilder {
	b.rules = append(b.rules, r)
	return b
}

// Build creates the Checker from the accumulated rules.
func (b *CheckerBuilder) Build() *Checker {
	return &Checker{rules: b.rules}
}

// RuleFactory creates a Rule from a type name and config map.
func RuleFactory(typeName string, cat *catalog.Catalog, c *cache.Cache, config map[string]any) (rule.Rule, error) {
	switch typeName {
	case "ratelimit":
		return rule.NewRateLimit(cat, c, config)
	default:
		return nil, fmt.Errorf("risk: unknown rule type %q", typeName)
	}
}
