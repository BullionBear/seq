package msgbus

import (
	"strings"
	"testing"

	"github.com/BullionBear/seq/core/model/event"
)

func TestEventBus_AssertOrder_OK(t *testing.T) {
	bus := NewEventBus()
	bus.RegisterPhased(PhaseIngest, "orderbook", []event.Topic{event.TopicEventDepthUpdate}, func(Event) {})
	bus.RegisterPhased(PhaseOrder, "oms", []event.Topic{event.TopicEventOrderNew}, func(Event) {})
	bus.RegisterPhased(PhaseAccount, "inventory", []event.Topic{event.TopicEventBalanceUpdate}, func(Event) {})
	bus.RegisterPhased(PhaseDecide, "xarb", []event.Topic{event.TopicEventDepthUpdate}, func(Event) {})

	if err := bus.AssertOrder(); err != nil {
		t.Fatalf("AssertOrder: %v", err)
	}

	got := bus.ConsumerNames()
	want := []string{"orderbook", "oms", "inventory", "xarb"}
	if len(got) != len(want) {
		t.Fatalf("ConsumerNames len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ConsumerNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEventBus_AssertOrder_SamePhaseOK(t *testing.T) {
	bus := NewEventBus()
	bus.RegisterPhased(PhaseIngest, "orderbook-a", nil, func(Event) {})
	bus.RegisterPhased(PhaseIngest, "orderbook-b", nil, func(Event) {})
	if err := bus.AssertOrder(); err != nil {
		t.Fatalf("same-phase order should be allowed: %v", err)
	}
}

func TestEventBus_AssertOrder_Violation(t *testing.T) {
	bus := NewEventBus()
	bus.RegisterPhased(PhaseDecide, "strategy", nil, func(Event) {})
	bus.RegisterPhased(PhaseIngest, "orderbook", nil, func(Event) {})

	err := bus.AssertOrder()
	if err == nil {
		t.Fatal("AssertOrder: expected error for phase regression")
	}
	if !strings.Contains(err.Error(), "orderbook") {
		t.Fatalf("error should name offending consumer: %v", err)
	}
	if !strings.Contains(err.Error(), "CONSUMER_ORDER.md") {
		t.Fatalf("error should point at docs: %v", err)
	}
}

func TestMsgBus_AssertOrder(t *testing.T) {
	bus := NewMsgBus()
	bus.RegisterPhased(PhaseIngest, "a", nil, func(Event) {})
	bus.RegisterPhased(PhaseDecide, "b", nil, func(Event) {})
	if err := bus.AssertOrder(); err != nil {
		t.Fatalf("AssertOrder: %v", err)
	}
	if got := bus.ConsumerNames(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ConsumerNames = %v", got)
	}
}
