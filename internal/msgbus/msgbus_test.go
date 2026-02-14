package msgbus

import (
	"testing"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// ============================================================================
// Tests for Command Serialization
// ============================================================================

func TestSerializeDeserializeOrderSubmitCommand(t *testing.T) {
	cmd := event.OrderSubmitCommand{
		AccountID:   1,
		SymbolID:    42,
		Side:        common.SideBuy,
		OrderType:   common.OrderTypeLimit,
		TimeInForce: common.TimeInForceGTC,
		Price:       50000.0,
		Quantity:    1.5,
	}

	size := OrderSubmitCommandSize()
	buf := make([]byte, size)

	written := SerializeOrderSubmitCommand(buf, &cmd)
	if uint64(written) != size {
		t.Errorf("SerializeOrderSubmitCommand wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderSubmitCommand(buf)

	if result.AccountID != cmd.AccountID {
		t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, cmd.AccountID)
	}
	if result.SymbolID != cmd.SymbolID {
		t.Errorf("SymbolID mismatch: got %d, want %d", result.SymbolID, cmd.SymbolID)
	}
	if result.Side != cmd.Side {
		t.Errorf("Side mismatch: got %d, want %d", result.Side, cmd.Side)
	}
	if result.OrderType != cmd.OrderType {
		t.Errorf("OrderType mismatch: got %d, want %d", result.OrderType, cmd.OrderType)
	}
	if result.TimeInForce != cmd.TimeInForce {
		t.Errorf("TimeInForce mismatch: got %d, want %d", result.TimeInForce, cmd.TimeInForce)
	}
	if result.Price != cmd.Price {
		t.Errorf("Price mismatch: got %f, want %f", result.Price, cmd.Price)
	}
	if result.Quantity != cmd.Quantity {
		t.Errorf("Quantity mismatch: got %f, want %f", result.Quantity, cmd.Quantity)
	}
}

func TestSerializeDeserializeOrderCancelCommand(t *testing.T) {
	cmd := event.OrderCancelCommand{
		AccountID:     1,
		ClientOrderID: 42,
	}

	size := OrderCancelCommandSize()
	buf := make([]byte, size)

	written := SerializeOrderCancelCommand(buf, &cmd)
	if uint64(written) != size {
		t.Errorf("SerializeOrderCancelCommand wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderCancelCommand(buf)

	if result.AccountID != cmd.AccountID {
		t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, cmd.AccountID)
	}
	if result.ClientOrderID != cmd.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, cmd.ClientOrderID)
	}
}

func TestSerializeDeserializeCancelAllCommand(t *testing.T) {
	cmd := event.CancelAllCommand{
		AccountID: 1,
		SymbolID:  42,
	}

	size := CancelAllCommandSize()
	buf := make([]byte, size)

	written := SerializeCancelAllCommand(buf, &cmd)
	if uint64(written) != size {
		t.Errorf("SerializeCancelAllCommand wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeCancelAllCommand(buf)

	if result.AccountID != cmd.AccountID {
		t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, cmd.AccountID)
	}
	if result.SymbolID != cmd.SymbolID {
		t.Errorf("SymbolID mismatch: got %d, want %d", result.SymbolID, cmd.SymbolID)
	}
}

// ============================================================================
// Tests for MsgBus Creation
// ============================================================================

func TestNewMsgBus(t *testing.T) {
	bus := NewMsgBus()
	if bus == nil {
		t.Fatal("NewMsgBus returned nil")
	}
	if bus.ConsumerCount() != 0 {
		t.Errorf("Expected 0 consumers, got %d", bus.ConsumerCount())
	}
}

// ============================================================================
// Tests for Command Channel
// ============================================================================

func TestMsgBus_SendAndDispatchCommand(t *testing.T) {
	bus := NewMsgBus()

	// Track command handling
	var handledCmd Command
	handled := false

	// Register command handler
	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {
		handledCmd = cmd
		handled = true
	})

	// Send a command
	cmd := event.OrderSubmitCommand{
		AccountID: 1,
		SymbolID:  42,
		Side:      common.SideBuy,
		OrderType: common.OrderTypeLimit,
		Price:     50000.0,
		Quantity:  1.5,
	}
	size := OrderSubmitCommandSize()
	offset, buf := bus.AllocateCmd(size)
	SerializeOrderSubmitCommand(buf, &cmd)
	bus.Send(CommandRef{
		Topic:  event.TopicCommandOrderSubmit,
		Index:  offset,
		Length: size,
	})

	// Dispatch should process the command
	hasWork := bus.Dispatch()
	if !hasWork {
		t.Error("Expected Dispatch to return true")
	}
	if !handled {
		t.Fatal("Expected command handler to be called")
	}
	if handledCmd.Ref.Topic != event.TopicCommandOrderSubmit {
		t.Errorf("Expected topic %d, got %d", event.TopicCommandOrderSubmit, handledCmd.Ref.Topic)
	}

	// Verify command payload
	cmdBuf := bus.ReadCmdBuffer(handledCmd.Ref.Index, handledCmd.Ref.Length)
	result := DeserializeOrderSubmitCommand(cmdBuf)
	if result.AccountID != 1 {
		t.Errorf("Expected AccountID 1, got %d", result.AccountID)
	}
	if result.Price != 50000.0 {
		t.Errorf("Expected Price 50000.0, got %f", result.Price)
	}
}

func TestMsgBus_CommandPriorityOverEvent(t *testing.T) {
	bus := NewMsgBus()

	// Track order of processing
	var order []string

	// Register event consumer
	bus.Register("test-consumer", nil, func(ev Event) {
		order = append(order, "event")
	})

	// Register command handler
	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {
		order = append(order, "command")
	})

	// Publish an event first
	size := TickSize()
	offset, buf := bus.Allocate(size)
	SerializeTick(buf, &event.Tick{SymbolID: 1, Price: 50000.0})
	bus.Publish(EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})

	// Then send a command
	cmdSize := OrderSubmitCommandSize()
	cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
	SerializeOrderSubmitCommand(cmdBuf, &event.OrderSubmitCommand{
		AccountID: 1,
		SymbolID:  42,
	})
	bus.Send(CommandRef{
		Topic:  event.TopicCommandOrderSubmit,
		Index:  cmdOffset,
		Length: cmdSize,
	})

	// Dispatch should process command FIRST, then event
	bus.Dispatch()

	if len(order) != 2 {
		t.Fatalf("Expected 2 items processed, got %d", len(order))
	}
	if order[0] != "command" {
		t.Errorf("Expected command to be processed first, got %q", order[0])
	}
	if order[1] != "event" {
		t.Errorf("Expected event to be processed second, got %q", order[1])
	}
}

func TestMsgBus_MultipleCommandsDrainedBeforeEvent(t *testing.T) {
	bus := NewMsgBus()

	cmdCount := 0
	eventCount := 0

	// Register event consumer
	bus.Register("test-consumer", nil, func(ev Event) {
		eventCount++
	})

	// Register command handler
	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {
		cmdCount++
	})

	// Publish an event
	size := TickSize()
	offset, buf := bus.Allocate(size)
	SerializeTick(buf, &event.Tick{SymbolID: 1})
	bus.Publish(EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})

	// Send 3 commands
	for i := 0; i < 3; i++ {
		cmdSize := OrderSubmitCommandSize()
		cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
		SerializeOrderSubmitCommand(cmdBuf, &event.OrderSubmitCommand{AccountID: i})
		bus.Send(CommandRef{
			Topic:  event.TopicCommandOrderSubmit,
			Index:  cmdOffset,
			Length: cmdSize,
		})
	}

	// Single Dispatch call should drain all 3 commands + 1 event
	bus.Dispatch()

	if cmdCount != 3 {
		t.Errorf("Expected 3 commands processed, got %d", cmdCount)
	}
	if eventCount != 1 {
		t.Errorf("Expected 1 event processed, got %d", eventCount)
	}
}

func TestMsgBus_RegisterCommandDuplicate(t *testing.T) {
	bus := NewMsgBus()

	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {})

	// Registering duplicate should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on duplicate command registration")
		}
	}()

	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {})
}

func TestMsgBus_DispatchEmptyReturns_false(t *testing.T) {
	bus := NewMsgBus()
	hasWork := bus.Dispatch()
	if hasWork {
		t.Error("Expected Dispatch to return false when empty")
	}
}

func TestMsgBus_EventDispatchDelegation(t *testing.T) {
	bus := NewMsgBus()

	var receivedTopic event.Topic
	bus.Register("test", []event.Topic{event.TopicEventTick}, func(ev Event) {
		receivedTopic = ev.Ref.Topic
	})

	size := TickSize()
	offset, buf := bus.Allocate(size)
	SerializeTick(buf, &event.Tick{SymbolID: 1, Price: 100.0})
	bus.Publish(EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})

	bus.Dispatch()

	if receivedTopic != event.TopicEventTick {
		t.Errorf("Expected topic %d, got %d", event.TopicEventTick, receivedTopic)
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkMsgBus_SendCommand(b *testing.B) {
	bus := NewMsgBus()
	bus.RegisterCommand(event.TopicCommandOrderSubmit, func(cmd Command) {})

	cmd := event.OrderSubmitCommand{
		AccountID: 1,
		SymbolID:  42,
		Side:      common.SideBuy,
		Price:     50000.0,
		Quantity:  1.5,
	}
	cmdSize := OrderSubmitCommandSize()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
		SerializeOrderSubmitCommand(cmdBuf, &cmd)
		bus.Send(CommandRef{
			Topic:  event.TopicCommandOrderSubmit,
			Index:  cmdOffset,
			Length: cmdSize,
		})
		bus.Dispatch()
	}
}

func BenchmarkMsgBus_DispatchEvent(b *testing.B) {
	bus := NewMsgBus()
	bus.Register("bench", nil, func(ev Event) {})

	tick := event.Tick{SymbolID: 1, Price: 50000.0}
	tickSize := TickSize()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		offset, buf := bus.Allocate(tickSize)
		SerializeTick(buf, &tick)
		bus.Publish(EventRef{
			Topic:  event.TopicEventTick,
			Index:  offset,
			Length: tickSize,
		})
		bus.Dispatch()
		bus.Release()
	}
}
