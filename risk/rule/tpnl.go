package rule

import (
	"fmt"
	"time"

	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/mitchellh/mapstructure"
)

// TpnlStop rejects orders when the trading PnL for a given (account, window)
// pair drops below the configured stop-loss threshold. The corresponding TPNL
// actor must be running with matching account and window parameters.
type TpnlStop struct {
	cache     *cache.Cache
	accountID int     // -1 = all accounts
	cacheKey  string  // links to the matching TPNL actor
	stopLoss  float64 // must be < 0
}

type tpnlStopConfig struct {
	Account  string  `yaml:"account"`   // account name, empty = all
	Window   string  `yaml:"window"`    // must match the actor's window
	StopLoss float64 `yaml:"stop_loss"` // must be < 0
}

// NewTpnlStop creates a TpnlStop rule from config.
func NewTpnlStop(cat *catalog.Catalog, c *cache.Cache, config map[string]any) (Rule, error) {
	var cfg tpnlStopConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		return nil, fmt.Errorf("tpnl: failed to create config decoder: %w", err)
	}
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("tpnl: failed to decode config: %w", err)
	}

	if cfg.StopLoss >= 0 {
		return nil, fmt.Errorf("tpnl: stop_loss must be < 0, got %f", cfg.StopLoss)
	}
	if cfg.Window == "" {
		return nil, fmt.Errorf("tpnl: window is required")
	}
	dur, err := time.ParseDuration(cfg.Window)
	if err != nil {
		return nil, fmt.Errorf("tpnl: invalid window duration %q: %w", cfg.Window, err)
	}
	if dur <= 0 {
		return nil, fmt.Errorf("tpnl: window must be > 0")
	}

	accountID := -1
	if cfg.Account != "" {
		acct := cat.GetAccountByName(cfg.Account)
		if acct == nil {
			return nil, fmt.Errorf("tpnl: account %q not found", cfg.Account)
		}
		accountID = acct.ID
	}

	return &TpnlStop{
		cache:     c,
		accountID: accountID,
		cacheKey:  cache.TpnlCacheKey(accountID, uint64(dur)),
		stopLoss:  cfg.StopLoss,
	}, nil
}

func (r *TpnlStop) Check(cmd command.RiskCheck) error {
	if r.accountID != -1 && cmd.AccountID != r.accountID {
		return nil
	}

	tpnl := r.cache.GetTpnl(r.cacheKey)

	if tpnl < r.stopLoss {
		log().Warn().
			Int("account_id", r.accountID).
			Int("client_order_id", cmd.ClientOrderID).
			Float64("tpnl", tpnl).
			Float64("stop_loss", r.stopLoss).
			Msg("TpnlStop: rejected, stop loss breached")
		return fmt.Errorf("tpnl stop loss breached: pnl %.4f < threshold %.4f", tpnl, r.stopLoss)
	}

	log().Debug().
		Int("account_id", r.accountID).
		Int("client_order_id", cmd.ClientOrderID).
		Float64("tpnl", tpnl).
		Float64("stop_loss", r.stopLoss).
		Msg("TpnlStop: passed")
	return nil
}
