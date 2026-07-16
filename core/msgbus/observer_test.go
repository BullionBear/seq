package msgbus

import (
	"context"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

// TestUnroutedCommandCounted verifies that a command with no registered
// processor increments the counter (P2-3: counted, not logged inline).
func TestUnroutedCommandCounted(t *testing.T) {
	bus := NewMsgBus()

	ref, buf := bus.AllocateCmd(command.CommandTypeCancelAll, 8)
	copy(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	bus.Send(ref)

	if !bus.Dispatch() {
		t.Fatal("expected Dispatch to process the pending command")
	}
	if got := bus.UnroutedCommandCount(); got != 1 {
		t.Fatalf("UnroutedCommandCount = %d, want 1", got)
	}
}

// TestObserverLifecycle verifies the observer samples counters and stops on
// context cancellation without touching hot-path state.
func TestObserverLifecycle(t *testing.T) {
	bus := NewMsgBus()
	ctx, cancel := context.WithCancel(context.Background())

	bus.StartObserver(ctx, 10*time.Millisecond)

	// Generate an unrouted command so a delta exists for at least one sample.
	ref, _ := bus.AllocateCmd(command.CommandTypeCancelAll, 8)
	bus.Send(ref)
	bus.Dispatch()

	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond) // let the goroutine observe cancellation

	// Counters remain readable and consistent after shutdown.
	if got := bus.UnroutedCommandCount(); got != 1 {
		t.Fatalf("UnroutedCommandCount = %d, want 1", got)
	}
	var snap observerSnapshot
	bus.sampleCounters(&snap)
	if snap.unrouted != 1 {
		t.Fatalf("sampleCounters unrouted = %d, want 1", snap.unrouted)
	}
	for topic := event.Topic(0); topic < event.TopicCount; topic++ {
		if snap.drops[topic] != 0 {
			t.Fatalf("unexpected drops for topic %s", topic)
		}
	}
}
