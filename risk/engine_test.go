package risk

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

func registerTestFactory(t *testing.T, typeName string, factory Factory) {
	t.Helper()
	Register(typeName, factory)
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, typeName)
		registryMu.Unlock()
	})
}

type stubActor struct {
	actor.ActorBase
	checkFn func(command.RiskCheck) error
	handled atomic.Int32
}

func (s *stubActor) Check(cmd command.RiskCheck) error {
	if s.checkFn == nil {
		return nil
	}
	return s.checkFn(cmd)
}

func (s *stubActor) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	s.handled.Add(1)
}

func TestEngine_Init_UnknownActorType(t *testing.T) {
	e := NewEngine(&catalog.Catalog{}, msgbus.NewMsgBus(), cache.NewCache())
	err := e.Init(Config{
		Actor: []actor.Entry{{Type: "no-such-type", Name: "bad"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown actor type")
	}
}

func TestEngine_Init_StatelessGuardNotRegistered(t *testing.T) {
	const typeName = "test-stateless-guard"
	registerTestFactory(t, typeName, func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return &stubActor{
			ActorBase: actor.NewActorBase("stateless", nil),
			checkFn:   func(command.RiskCheck) error { return nil },
		}
	})

	bus := msgbus.NewMsgBus()
	before := bus.ConsumerCount()
	e := NewEngine(&catalog.Catalog{}, bus, cache.NewCache())
	if err := e.Init(Config{
		Actor: []actor.Entry{{Type: typeName, Name: "g1"}},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := bus.ConsumerCount(); got != before {
		t.Fatalf("ConsumerCount = %d, want %d (stateless guard must not register)", got, before)
	}
	if len(e.guards) != 1 {
		t.Fatalf("guards = %d, want 1", len(e.guards))
	}
}

func TestEngine_GuardOrdering(t *testing.T) {
	const (
		typeReject = "test-guard-reject"
		typeSecond = "test-guard-second"
	)
	var secondCalls atomic.Int32

	registerTestFactory(t, typeReject, func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return &stubActor{
			ActorBase: actor.NewActorBase("rejector", nil),
			checkFn:   func(command.RiskCheck) error { return errors.New("reject") },
		}
	})
	registerTestFactory(t, typeSecond, func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return &stubActor{
			ActorBase: actor.NewActorBase("second", nil),
			checkFn: func(command.RiskCheck) error {
				secondCalls.Add(1)
				return nil
			},
		}
	})

	e := NewEngine(&catalog.Catalog{}, msgbus.NewMsgBus(), cache.NewCache())
	if err := e.Init(Config{
		Actor: []actor.Entry{
			{Type: typeReject, Name: "first"},
			{Type: typeSecond, Name: "second"},
		},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	name, err := e.riskCheck(command.RiskCheck{AccountID: 1})
	if err == nil {
		t.Fatal("expected reject from first guard")
	}
	if name != "first" {
		t.Fatalf("guard name = %q, want first", name)
	}
	if secondCalls.Load() != 0 {
		t.Fatalf("second guard calls = %d, want 0 (short-circuit)", secondCalls.Load())
	}
}

func TestEngine_Init_SubscribedGuardIsRegistered(t *testing.T) {
	const typeName = "test-subscribed-guard"
	registerTestFactory(t, typeName, func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return &stubActor{
			ActorBase: actor.NewActorBase("subscribed", []event.Topic{event.TopicEventTimer}),
			checkFn:   func(command.RiskCheck) error { return nil },
		}
	})

	bus := msgbus.NewMsgBus()
	before := bus.ConsumerCount()
	e := NewEngine(&catalog.Catalog{}, bus, cache.NewCache())
	if err := e.Init(Config{
		Actor: []actor.Entry{{Type: typeName, Name: "sub"}},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := bus.ConsumerCount(); got != before+1 {
		t.Fatalf("ConsumerCount = %d, want %d", got, before+1)
	}
}

func TestEngine_RiskCheckMsgPrefix(t *testing.T) {
	const typeName = "test-rate-guard"
	registerTestFactory(t, typeName, func(cat *catalog.Catalog, bus *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return &stubActor{
			ActorBase: actor.NewActorBase("rate", nil),
			checkFn:   func(command.RiskCheck) error { return RateLimited(42) },
		}
	})

	e := NewEngine(&catalog.Catalog{}, msgbus.NewMsgBus(), cache.NewCache())
	if err := e.Init(Config{
		Actor: []actor.Entry{{Type: typeName, Name: "rate-limiter-bybit"}},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	name, err := e.riskCheck(command.RiskCheck{})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := name + ": " + err.Error()
	want := "rate-limiter-bybit: rate limited: next accepted in 42 ms"
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}
	if CodeOf(err) != ErrCodeRateLimited {
		t.Fatalf("code = %d, want %d", CodeOf(err), ErrCodeRateLimited)
	}
}
