package ratelimiter

import (
	"testing"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/risk"
)

func newTestBucket(t *testing.T, leakRate float64, capacity int, accountID int) *LeakyBucket {
	t.Helper()
	lb := NewLeakyBucket(&catalog.Catalog{}, nil, nil)
	lb.OnInit(map[string]any{
		"leak_rate": leakRate,
		"capacity":  capacity,
	})
	lb.accountID = accountID
	return lb
}

func TestLeakyBucket_BurstWithinSingleDispatch(t *testing.T) {
	const capacity = 4
	lb := newTestBucket(t, 100, capacity, -1)
	const t0 uint64 = 1_000_000_000
	cmd := command.RiskCheck{AccountID: 1, Timestamp: t0}

	for i := 0; i < capacity; i++ {
		cmd.ClientOrderID = i + 1
		if err := lb.Check(cmd); err != nil {
			t.Fatalf("check %d: unexpected reject: %v", i+1, err)
		}
	}

	cmd.ClientOrderID = capacity + 1
	err := lb.Check(cmd)
	if err == nil {
		t.Fatal("expected capacity+1 check to be rejected")
	}
	if got := risk.CodeOf(err); got != risk.ErrCodeRateLimited {
		t.Fatalf("CodeOf = %d, want %d", got, risk.ErrCodeRateLimited)
	}
}

func TestLeakyBucket_AccountScoping(t *testing.T) {
	lb := newTestBucket(t, 100, 1, 7)
	const t0 uint64 = 1_000_000_000

	if err := lb.Check(command.RiskCheck{AccountID: 99, ClientOrderID: 1, Timestamp: t0}); err != nil {
		t.Fatalf("other account should pass: %v", err)
	}
	if err := lb.Check(command.RiskCheck{AccountID: 99, ClientOrderID: 2, Timestamp: t0}); err != nil {
		t.Fatalf("other account should not consume capacity: %v", err)
	}

	if err := lb.Check(command.RiskCheck{AccountID: 7, ClientOrderID: 3, Timestamp: t0}); err != nil {
		t.Fatalf("scoped account first check: %v", err)
	}
	if err := lb.Check(command.RiskCheck{AccountID: 7, ClientOrderID: 4, Timestamp: t0}); err == nil {
		t.Fatal("scoped account second check should be rate limited")
	}

	global := newTestBucket(t, 100, 1, -1)
	if err := global.Check(command.RiskCheck{AccountID: 1, ClientOrderID: 1, Timestamp: t0}); err != nil {
		t.Fatalf("global first: %v", err)
	}
	if err := global.Check(command.RiskCheck{AccountID: 2, ClientOrderID: 2, Timestamp: t0}); err == nil {
		t.Fatal("global bucket should apply to all accounts")
	}
}

func TestLeakyBucket_LeakRate(t *testing.T) {
	// capacity=2, leak_rate=100 → window = 20ms
	lb := newTestBucket(t, 100, 2, -1)
	const t0 uint64 = 1_000_000_000
	cmd := command.RiskCheck{AccountID: 1, Timestamp: t0}

	if err := lb.Check(cmd); err != nil {
		t.Fatalf("admit 1: %v", err)
	}
	if err := lb.Check(cmd); err != nil {
		t.Fatalf("admit 2: %v", err)
	}
	if err := lb.Check(cmd); err == nil {
		t.Fatal("expected reject while bucket full")
	}

	cmd.Timestamp = t0 + lb.windowNs
	if err := lb.Check(cmd); err != nil {
		t.Fatalf("expected admit after leak window: %v", err)
	}
}

func TestLeakyBucket_LogicalCapacity(t *testing.T) {
	// Ring buffer rounds up to power-of-two (3 → 4); logical capacity must still be 3.
	const capacity = 3
	lb := newTestBucket(t, 100, capacity, -1)
	const t0 uint64 = 1_000_000_000
	cmd := command.RiskCheck{AccountID: 1, Timestamp: t0}

	for i := 0; i < capacity; i++ {
		cmd.ClientOrderID = i + 1
		if err := lb.Check(cmd); err != nil {
			t.Fatalf("check %d: unexpected reject: %v", i+1, err)
		}
	}
	cmd.ClientOrderID = capacity + 1
	if err := lb.Check(cmd); err == nil {
		t.Fatal("expected reject at logical capacity+1 (not ring power-of-two size)")
	}
}
