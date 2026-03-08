package node_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/catalog"
	coreconfig "github.com/BullionBear/seq/core/config"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/node"

	_ "github.com/BullionBear/seq/portfolio/actor/balance"
)

func TestIntegration_BalanceActorSnapshotOnStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := coreconfig.LoadConfig("../config/test.yml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if err := logger.Init(cfg.Logger.ToOptions()); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	cat := catalog.NewCatalog(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	if cat == nil {
		t.Fatal("Failed to initialize catalog")
	}

	n := node.NewNode(cat)

	if cfg.MsgBus.MsgLog.Enabled && cfg.MsgBus.MsgLog.Dir != "" {
		msgLogger, err := msgbus.NewMsgLogger(cfg.MsgBus.MsgLog.Dir)
		if err != nil {
			t.Fatalf("Failed to initialize message logger: %v", err)
		}
		defer msgLogger.Close()
		n.MsgBus().SetMsgLogger(msgLogger)
	}

	n.Init(cfg.Node, cfg.ExecRouter, cfg.DataRouter)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := n.ExecutionRouter().Connect(ctx); err != nil {
		t.Fatalf("Failed to connect execution router: %v", err)
	}
	defer n.ExecutionRouter().Disconnect()

	time.Sleep(2 * time.Second)

	n.PortfolioEngine().Start()
	t.Log("Portfolio engine started, waiting for balance snapshot...")

	bus := n.MsgBus()
	snapshotReceived := make(chan struct{}, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				hasWork := bus.Dispatch()
				if hasWork {
					bus.Release()
					bus.ReleaseArenas()
				} else {
					runtime.Gosched()
				}
			}
		}
	}()

	go func() {
		accounts := n.PortfolioEngine().GetConfiguredAccounts()
		if len(accounts) == 0 {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			default:
				allReady := true
				for _, acctID := range accounts {
					if n.Cache().GetAccountBalances(acctID) == nil {
						allReady = false
						break
					}
				}
				if allReady {
					select {
					case snapshotReceived <- struct{}{}:
					default:
					}
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	select {
	case <-snapshotReceived:
		t.Log("Balance snapshot received successfully!")
	case <-time.After(30 * time.Second):
		t.Fatal("Timed out waiting for balance snapshot")
	}

	for _, acctID := range n.PortfolioEngine().GetConfiguredAccounts() {
		balances := n.Cache().GetAccountBalances(acctID)
		t.Logf("Account %d: %d token balances", acctID, len(balances))
		for _, b := range balances {
			if b.Total > 0 {
				tokenName := "unknown"
				if tok, err := cat.GetToken(b.TokenID); err == nil {
					tokenName = tok.Name
				}
				t.Logf("  %s (ID=%d): Available=%.8f, Locked=%.8f, Total=%.8f",
					tokenName, b.TokenID, b.Available, b.Locked, b.Total)
			}
		}
	}
}

// TestIntegration_BalanceActorEventRouting verifies the balance actor correctly
// routes RespBalanceSnapshot events from the MsgBus to the cache.
func TestIntegration_BalanceActorEventRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := coreconfig.LoadConfig("../config/test.yml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if err := logger.Init(cfg.Logger.ToOptions()); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	cat := catalog.NewCatalog(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	if cat == nil {
		t.Fatal("Failed to initialize catalog")
	}

	n := node.NewNode(cat)
	n.Init(cfg.Node, cfg.ExecRouter, cfg.DataRouter)

	bus := n.MsgBus()
	acctID := n.PortfolioEngine().GetConfiguredAccounts()[0]

	// Resolve the walletID for the first configured wallet so the balance actor accepts the event
	wallet, err := cat.GetWalletByName("bybit-hephe-unified")
	if err != nil {
		t.Fatalf("Failed to resolve wallet: %v", err)
	}

	fakeSnapshot := event.RespBalanceSnapshot{
		AccountID: acctID,
		WalletID:  wallet.ID,
		Balances: []common.Balance{
			{TokenID: 1, Available: 100.5, Locked: 10.0, Total: 110.5},
			{TokenID: 2, Available: 50000.0, Locked: 0.0, Total: 50000.0},
		},
	}

	bufLen := uint64(fakeSnapshot.GetBufferLength())
	offset, buf := bus.Allocate(bufLen)
	if err := fakeSnapshot.Encode(buf); err != nil {
		t.Fatalf("Failed to encode snapshot: %v", err)
	}
	bus.Publish(msgbus.EventRef{
		Topic:  event.TopicEventRespBalanceSnapshot,
		Index:  offset,
		Length: bufLen,
	})

	for i := 0; i < 100; i++ {
		hasWork := bus.Dispatch()
		if hasWork {
			bus.Release()
			bus.ReleaseArenas()
		}
		if n.Cache().GetBalance(acctID, 1) != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	b1 := n.Cache().GetBalance(acctID, 1)
	if b1 == nil {
		t.Fatal("Expected balance for tokenID=1, got nil")
	}
	if b1.Available != 100.5 || b1.Locked != 10.0 || b1.Total != 110.5 {
		t.Errorf("TokenID=1 balance mismatch: got Available=%.2f, Locked=%.2f, Total=%.2f",
			b1.Available, b1.Locked, b1.Total)
	}

	b2 := n.Cache().GetBalance(acctID, 2)
	if b2 == nil {
		t.Fatal("Expected balance for tokenID=2, got nil")
	}
	if b2.Available != 50000.0 || b2.Total != 50000.0 {
		t.Errorf("TokenID=2 balance mismatch: got Available=%.2f, Total=%.2f",
			b2.Available, b2.Total)
	}

	balances := n.Cache().GetAccountBalances(acctID)
	t.Logf("Event routing verified: Account %d has %d token balances", acctID, len(balances))
}
