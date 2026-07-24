package ratelimiter

import (
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/risk"
	"github.com/mitchellh/mapstructure"
)

func init() {
	risk.Register("leakybucket", func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewLeakyBucket(cat, bus, c)
	})
}

var (
	_ actor.Actor = (*LeakyBucket)(nil)
	_ risk.Guard  = (*LeakyBucket)(nil)
)

// LeakyBucket is a rate-limiter Guard that tracks admitted order timestamps
// in a circular buffer. Check atomically admits (consumes a slot) or rejects.
//
// Counting admitted RiskCheck passes (not published OrderNew events) is
// intentional: if execution later drops the order (paper mode, venue reject),
// usage is overestimated. That bias is correct for protecting exchange API
// rate limits.
//
// SubscribedTypes is nil — this actor does not consume events. Risk Engine.Init
// skips msgbus registration for nil/empty topics (unlike core/actor's default
// of "subscribe all").
type LeakyBucket struct {
	actor.ActorBase
	catalog *catalog.Catalog

	// Circular buffer of admit timestamps (unix nanos).
	// Acts as the "bucket" -- logical capacity equals max burst size.
	bucket *mem.SPSCRingBuffer[uint64]

	leakRate  float64 // orders per second
	capacity  uint64
	accountID int // -1 = global (all accounts)

	// Derived: full window in nanoseconds (capacity / leakRate * 1e9)
	windowNs uint64
}

// NewLeakyBucket creates a new LeakyBucket actor with no event subscriptions.
func NewLeakyBucket(cat *catalog.Catalog, _ *msgbus.MsgBus, _ *cache.Cache) *LeakyBucket {
	return &LeakyBucket{
		ActorBase: actor.NewActorBase("leakybucket", nil),
		catalog:   cat,
	}
}

type leakyBucketConfig struct {
	LeakRate float64 `yaml:"leak_rate"`
	Capacity int     `yaml:"capacity"`
	Account  string  `yaml:"account"` // account name, empty = global (all accounts)
}

func (lb *LeakyBucket) OnInit(config map[string]any) {
	var cfg leakyBucketConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		lb.Log().Fatal().Err(err).Msg("LeakyBucket: failed to create config decoder")
	}
	if err := decoder.Decode(config); err != nil {
		lb.Log().Fatal().Err(err).Msg("LeakyBucket: failed to decode config")
	}

	if cfg.LeakRate <= 0 {
		lb.Log().Fatal().Float64("leak_rate", cfg.LeakRate).Msg("LeakyBucket: leak_rate must be > 0")
	}
	if cfg.Capacity <= 0 {
		lb.Log().Fatal().Int("capacity", cfg.Capacity).Msg("LeakyBucket: capacity must be > 0")
	}

	lb.accountID = -1
	if cfg.Account != "" {
		acct := lb.catalog.GetAccountByName(cfg.Account)
		if acct == nil {
			lb.Log().Fatal().Str("account", cfg.Account).Msg("LeakyBucket: account not found")
		}
		lb.accountID = acct.ID
	}

	lb.leakRate = cfg.LeakRate
	lb.capacity = uint64(cfg.Capacity)
	lb.windowNs = uint64(float64(cfg.Capacity) / cfg.LeakRate * float64(time.Second))
	lb.bucket = mem.NewSPSCRingBuffer[uint64](lb.capacity)

	lb.Log().Info().
		Float64("leak_rate", lb.leakRate).
		Uint64("capacity", lb.capacity).
		Int("account_id", lb.accountID).
		Str("account", cfg.Account).
		Msg("LeakyBucket: initialized")
}

// Check admits the order into the rate bucket or rejects when the burst
// capacity within the sliding window is exhausted.
//
// The sliding window is driven by cmd.Timestamp (set by SubmitOrder), not
// wall-clock time.Now(). That keeps admit/drain deterministic and aligned
// with the order's CreatedAt, independent of actor.Register / Clock injection.
func (lb *LeakyBucket) Check(cmd command.RiskCheck) error {
	if lb.accountID != -1 && cmd.AccountID != lb.accountID {
		return nil
	}

	now := cmd.Timestamp
	if now == 0 {
		now = uint64(time.Now().UnixNano())
	}
	lb.drainExpired(now)

	// Enforce configured capacity (ring buffer size may be rounded up to
	// the next power of two).
	if lb.bucket.Count() >= lb.capacity {
		oldest, _ := lb.bucket.Peek()
		waitMs := (oldest + lb.windowNs - now) / 1e6
		lb.Log().Debug().
			Int("account_id", lb.accountID).
			Int("client_order_id", cmd.ClientOrderID).
			Uint64("bucket_count", lb.bucket.Count()).
			Uint64("wait_ms", waitMs).
			Msg("LeakyBucket: rejected")
		return risk.RateLimited(waitMs)
	}

	if !lb.bucket.Write(now) {
		oldest, _ := lb.bucket.Peek()
		waitMs := (oldest + lb.windowNs - now) / 1e6
		return risk.RateLimited(waitMs)
	}

	lb.Log().Debug().
		Int("account_id", lb.accountID).
		Int("client_order_id", cmd.ClientOrderID).
		Uint64("bucket_count", lb.bucket.Count()).
		Msg("LeakyBucket: admitted")
	return nil
}

func (lb *LeakyBucket) drainExpired(now uint64) {
	if now < lb.windowNs {
		return
	}
	cutoff := now - lb.windowNs
	for {
		ts, ok := lb.bucket.Peek()
		if !ok || ts > cutoff {
			break
		}
		lb.bucket.Read()
	}
}
