package msgbus

import (
	"testing"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// ============================================================================
// Tests for Command Encode/Decode
// ============================================================================

func TestEncodeDecodeSubmitOrder(t *testing.T) {
	cmd := command.SubmitOrder{
		AccountID:   1,
		SymbolID:    42,
		Side:        common.SideBuy,
		OrderType:   common.OrderTypeLimit,
		TimeInForce: common.TimeInForceGTC,
		Price:       50000.0,
		Quantity:    1.5,
	}

	size := cmd.GetBufferLength()
	buf := make([]byte, size)

	if err := cmd.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	result := command.NewSubmitOrderFromBytes(buf)

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

func TestEncodeDecodeCancelOrder(t *testing.T) {
	cmd := command.CancelOrder{
		AccountID:     1,
		ClientOrderID: 42,
	}

	size := cmd.GetBufferLength()
	buf := make([]byte, size)

	if err := cmd.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	result := command.NewCancelOrderFromBytes(buf)

	if result.AccountID != cmd.AccountID {
		t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, cmd.AccountID)
	}
	if result.ClientOrderID != cmd.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, cmd.ClientOrderID)
	}
}

func TestEncodeDecodeCancelAll(t *testing.T) {
	cmd := command.CancelAll{
		AccountID: 1,
		SymbolID:  42,
	}

	size := cmd.GetBufferLength()
	buf := make([]byte, size)

	if err := cmd.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	result := command.NewCancelAllFromBytes(buf)

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
	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {
		handledCmd = cmd
		handled = true
	})

	// Send a command
	cmd := command.SubmitOrder{
		AccountID: 1,
		SymbolID:  42,
		Side:      common.SideBuy,
		OrderType: common.OrderTypeLimit,
		Price:     50000.0,
		Quantity:  1.5,
	}
	size := uint64(cmd.GetBufferLength())
	offset, buf := bus.AllocateCmd(size)
	cmd.Encode(buf)
	bus.Send(CommandRef{
		CommandType: command.CommandTypeOrderSubmit,
		Index:       offset,
		Length:      size,
	})

	// Dispatch should process the command
	hasWork := bus.Dispatch()
	if !hasWork {
		t.Error("Expected Dispatch to return true")
	}
	if !handled {
		t.Fatal("Expected command handler to be called")
	}
	if handledCmd.Ref.CommandType != command.CommandTypeOrderSubmit {
		t.Errorf("Expected command type %d, got %d", command.CommandTypeOrderSubmit, handledCmd.Ref.CommandType)
	}

	// Verify command payload
	cmdBuf := bus.ReadCmdBuffer(handledCmd.Ref.Index, handledCmd.Ref.Length)
	result := command.NewSubmitOrderFromBytes(cmdBuf)
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
	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {
		order = append(order, "command")
	})

	// Publish an event first
	tick := event.Tick{SymbolID: 1, Price: 50000.0}
	size := uint64(tick.GetBufferLength())
	offset, buf := bus.Allocate(size)
	tick.Encode(buf)
	bus.Publish(EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})

	// Then send a command
	submitCmd := command.SubmitOrder{
		AccountID: 1,
		SymbolID:  42,
	}
	cmdSize := uint64(submitCmd.GetBufferLength())
	cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
	submitCmd.Encode(cmdBuf)
	bus.Send(CommandRef{
		CommandType: command.CommandTypeOrderSubmit,
		Index:       cmdOffset,
		Length:      cmdSize,
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
	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {
		cmdCount++
	})

	// Publish an event
	tick := event.Tick{SymbolID: 1}
	size := uint64(tick.GetBufferLength())
	offset, buf := bus.Allocate(size)
	tick.Encode(buf)
	bus.Publish(EventRef{
		Topic:  event.TopicEventTick,
		Index:  offset,
		Length: size,
	})

	// Send 3 commands
	for i := 0; i < 3; i++ {
		submitCmd := command.SubmitOrder{AccountID: i}
		cmdSize := uint64(submitCmd.GetBufferLength())
		cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
		submitCmd.Encode(cmdBuf)
		bus.Send(CommandRef{
			CommandType: command.CommandTypeOrderSubmit,
			Index:       cmdOffset,
			Length:      cmdSize,
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

	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {})

	// Registering duplicate should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on duplicate command registration")
		}
	}()

	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {})
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

	tick := event.Tick{SymbolID: 1, Price: 100.0}
	size := uint64(tick.GetBufferLength())
	offset, buf := bus.Allocate(size)
	tick.Encode(buf)
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
	bus.RegisterCommand(command.CommandTypeOrderSubmit, func(cmd Command) {})

	cmd := command.SubmitOrder{
		AccountID: 1,
		SymbolID:  42,
		Side:      common.SideBuy,
		Price:     50000.0,
		Quantity:  1.5,
	}
	cmdSize := uint64(cmd.GetBufferLength())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmdOffset, cmdBuf := bus.AllocateCmd(cmdSize)
		cmd.Encode(cmdBuf)
		bus.Send(CommandRef{
			CommandType: command.CommandTypeOrderSubmit,
			Index:       cmdOffset,
			Length:      cmdSize,
		})
		bus.Dispatch()
	}
}

func BenchmarkMsgBus_DispatchEvent(b *testing.B) {
	bus := NewMsgBus()
	bus.Register("bench", nil, func(ev Event) {})

	tick := event.Tick{SymbolID: 1, Price: 50000.0}
	tickSize := uint64(tick.GetBufferLength())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		offset, buf := bus.Allocate(tickSize)
		tick.Encode(buf)
		bus.Publish(EventRef{
			Topic:  event.TopicEventTick,
			Index:  offset,
			Length: tickSize,
		})
		bus.Dispatch()
		bus.Release()
	}
}
