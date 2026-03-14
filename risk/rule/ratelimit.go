package rule

import (
	"fmt"
	"time"

	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/mitchellh/mapstructure"
)

// Rule is the interface that all risk rules must implement.
// Each rule inspects a RiskCheck command and returns an error to reject it,
// or nil to allow it through.
type Rule interface {
	Check(cmd command.RiskCheck) error
}

// RateLimit rejects orders when the current time is before the
// next-accepted time written by the leaky bucket actor.
type RateLimit struct {
	cache     *cache.Cache
	accountID int // -1 = global
}

type rateLimitConfig struct {
	AccountID int `yaml:"account_id"`
}

// NewRateLimit creates a RateLimit rule from config.
func NewRateLimit(c *cache.Cache, config map[string]any) (Rule, error) {
	var cfg rateLimitConfig
	cfg.AccountID = -1
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		return nil, fmt.Errorf("ratelimit: failed to create config decoder: %w", err)
	}
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("ratelimit: failed to decode config: %w", err)
	}
	return &RateLimit{
		cache:     c,
		accountID: cfg.AccountID,
	}, nil
}

func (r *RateLimit) Check(cmd command.RiskCheck) error {
	acctID := r.accountID
	if acctID != -1 && cmd.AccountID != acctID {
		return nil
	}

	nextAccepted := r.cache.GetRiskNextAcceptedTime(acctID)
	if nextAccepted == 0 {
		return nil
	}

	now := uint64(time.Now().UnixNano())
	if now < nextAccepted {
		return fmt.Errorf("rate limited: next accepted in %d ns", nextAccepted-now)
	}
	return nil
}
