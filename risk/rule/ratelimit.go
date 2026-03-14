package rule

import (
	"fmt"
	"time"

	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

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
	Account string `yaml:"account"` // account name, empty = global
}

// NewRateLimit creates a RateLimit rule from config.
func NewRateLimit(cat *catalog.Catalog, c *cache.Cache, config map[string]any) (Rule, error) {
	var cfg rateLimitConfig
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

	accountID := -1
	if cfg.Account != "" {
		acct := cat.GetAccountByName(cfg.Account)
		if acct == nil {
			return nil, fmt.Errorf("ratelimit: account %q not found", cfg.Account)
		}
		accountID = acct.ID
	}

	return &RateLimit{
		cache:     c,
		accountID: accountID,
	}, nil
}

func (r *RateLimit) Check(cmd command.RiskCheck) error {
	acctID := r.accountID
	if acctID != -1 && cmd.AccountID != acctID {
		return nil
	}

	nextAccepted := r.cache.GetRiskNextAcceptedTime(acctID)
	if nextAccepted == 0 {
		log().Debug().
			Int("account_id", acctID).
			Int("client_order_id", cmd.ClientOrderID).
			Msg("RateLimit: no rate data yet, allowing")
		return nil
	}

	now := uint64(time.Now().UnixNano())
	if now < nextAccepted {
		waitMs := (nextAccepted - now) / 1e6
		log().Debug().
			Int("account_id", acctID).
			Int("client_order_id", cmd.ClientOrderID).
			Uint64("wait_ms", waitMs).
			Msg("RateLimit: rejected")
		return fmt.Errorf("rate limited: next accepted in %d ms", waitMs)
	}

	log().Debug().
		Int("account_id", acctID).
		Int("client_order_id", cmd.ClientOrderID).
		Msg("RateLimit: passed")
	return nil
}
