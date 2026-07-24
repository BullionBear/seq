package ratelimiter

import (
	"testing"
	"time"

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
	cmd := command.RiskCheck{AccountID: 1, ClientOrderID: 1}

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

	if err := lb.Check(command.RiskCheck{AccountID: 99, ClientOrderID: 1}); err != nil {
		t.Fatalf("other account should pass: %v", err)
	}
	if err := lb.Check(command.RiskCheck{AccountID: 99, ClientOrderID: 2}); err != nil {
		t.Fatalf("other account should not consume capacity: %v", err)
	}

	if err := lb.Check(command.RiskCheck{AccountID: 7, ClientOrderID: 3}); err != nil {
		t.Fatalf("scoped account first check: %v", err)
	}
	if err := lb.Check(command.RiskCheck{AccountID: 7, ClientOrderID: 4}); err == nil {
		t.Fatal("scoped account second check should be rate limited")
	}

	global := newTestBucket(t, 100, 1, -1)
	if err := global.Check(command.RiskCheck{AccountID: 1, ClientOrderID: 1}); err != nil {
		t.Fatalf("global first: %v", err)
	}
	if err := global.Check(command.RiskCheck{AccountID: 2, ClientOrderID: 2}); err == nil {
		t.Fatal("global bucket should apply to all accounts")
	}
}

func TestLeakyBucket_LeakRate(t *testing.T) {
	// capacity=2, leak_rate=100 → window = 20ms
	lb := newTestBucket(t, 100, 2, -1)
	cmd := command.RiskCheck{AccountID: 1}

	if err := lb.Check(cmd); err != nil {
		t.Fatalf("admit 1: %v", err)
	}
	if err := lb.Check(cmd); err != nil {
		t.Fatalf("admit 2: %v", err)
	}
	if err := lb.Check(cmd); err == nil {
		t.Fatal("expected reject while bucket full")
	}

	time.Sleep(25 * time.Millisecond)

	if err := lb.Check(cmd); err != nil {
		t.Fatalf("expected admit after leak window: %v", err)
	}
}
