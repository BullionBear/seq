package ratelimiter

import (
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/risk"
	"github.com/mitchellh/mapstructure"
)

func init() {
	risk.Register("leakybucket", func(bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewLeakyBucket(bus, c)
	})
}

var _ actor.Actor = (*LeakyBucket)(nil)

// LeakyBucket is a rate-limiter actor that tracks order timestamps in a
// circular buffer and writes the next-accepted time to the shared cache.
type LeakyBucket struct {
	actor.ActorBase
	cache  *cache.Cache
	msgBus *msgbus.MsgBus

	// Circular buffer of order timestamps (unix nanos).
	// Acts as the "bucket" -- capacity equals max burst size.
	bucket *mem.SPSCRingBuffer[uint64]

	leakRate  float64 // orders per second
	capacity  uint64
	accountID int // -1 = global (all accounts)

	// Derived: nanoseconds between allowed requests (1 / leakRate * 1e9)
	intervalNs uint64
	// Derived: full window in nanoseconds (capacity / leakRate * 1e9)
	windowNs uint64
}

// NewLeakyBucket creates a new LeakyBucket actor with default subscriptions.
func NewLeakyBucket(bus *msgbus.MsgBus, c *cache.Cache) *LeakyBucket {
	return &LeakyBucket{
		ActorBase: actor.NewActorBase("leakybucket", []event.Topic{
			event.TopicEventOrderNew,
		}),
		cache:  c,
		msgBus: bus,
	}
}

type leakyBucketConfig struct {
	LeakRate  float64 `yaml:"leak_rate"`
	Capacity  int     `yaml:"capacity"`
	AccountID int     `yaml:"account_id"`
}

func (lb *LeakyBucket) OnInit(config map[string]any) {
	var cfg leakyBucketConfig
	cfg.AccountID = -1 // default
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

	lb.leakRate = cfg.LeakRate
	lb.capacity = uint64(cfg.Capacity)
	lb.accountID = cfg.AccountID
	lb.intervalNs = uint64(float64(time.Second) / cfg.LeakRate)
	lb.windowNs = uint64(float64(cfg.Capacity) / cfg.LeakRate * float64(time.Second))
	lb.bucket = mem.NewSPSCRingBuffer[uint64](lb.capacity)

	lb.Log().Info().
		Float64("leak_rate", lb.leakRate).
		Uint64("capacity", lb.capacity).
		Int("account_id", lb.accountID).
		Msg("LeakyBucket: initialized")
}

func (lb *LeakyBucket) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	if ev.Ref.Topic != event.TopicEventOrderNew {
		return
	}

	buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
	orderNew := event.NewOrderNewFromBytes(buf)

	if lb.accountID != -1 && orderNew.AccountID != lb.accountID {
		return
	}

	now := uint64(time.Now().UnixNano())
	cutoff := now - lb.windowNs

	// Drain expired entries from the front of the buffer.
	for {
		ts, ok := lb.bucket.Peek()
		if !ok || ts > cutoff {
			break
		}
		lb.bucket.Read()
	}

	// Push current timestamp. If bucket is full (burst exceeded), the oldest
	// entry was not yet expired -- compute next accepted time from it.
	if !lb.bucket.Write(now) {
		// Bucket full: next accepted = oldest entry + window duration.
		oldest, _ := lb.bucket.Peek()
		nextAccepted := oldest + lb.windowNs
		lb.cache.SetRiskNextAcceptedTime(lb.accountID, nextAccepted)
		return
	}

	// Bucket not full: next accepted = now + interval between requests.
	nextAccepted := now + lb.intervalNs
	lb.cache.SetRiskNextAcceptedTime(lb.accountID, nextAccepted)
}
