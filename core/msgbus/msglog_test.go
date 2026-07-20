package msgbus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

func TestMsgLog_WriteJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewMsgLogger(dir)
	if err != nil {
		t.Fatalf("NewMsgLogger: %v", err)
	}

	tick := event.Tick{SymbolID: 1, Price: 50000.5, Qty: 1.25, Timestamp: 1700000000000000000}
	tickBuf := make([]byte, tick.GetBufferLength())
	if err := tick.Encode(tickBuf); err != nil {
		t.Fatal(err)
	}
	logger.LogEvent(Event{
		Ref:       EventRef{Topic: event.TopicEventTick, Length: uint64(len(tickBuf))},
		EventID:   1,
		CreatedAt: 1700000000000000001,
	}, tickBuf)

	orderNew := event.OrderNew{AccountID: 9, ClientOrderID: 77, OrderID: -1, SymbolID: 1, Quantity: 2, Price: 49000}
	orderBuf := make([]byte, orderNew.GetBufferLength())
	if err := orderNew.Encode(orderBuf); err != nil {
		t.Fatal(err)
	}
	logger.LogEvent(Event{
		Ref:       EventRef{Topic: event.TopicEventOrderNew, Length: uint64(len(orderBuf))},
		EventID:   2,
		CreatedAt: 1700000000000000002,
	}, orderBuf)

	logger.LogEvent(Event{
		Ref:       EventRef{Topic: event.TopicEventTimer},
		EventID:   3,
		CreatedAt: 1700000000000000003,
	}, nil)

	// Truncated Tick payload: decoder must reject, logger must report inline.
	logger.LogEvent(Event{
		Ref:       EventRef{Topic: event.TopicEventTick, Length: 4},
		EventID:   4,
		CreatedAt: 1700000000000000004,
	}, tickBuf[:4])

	logger.Close()

	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "event_"+date+".jsonl"))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}

	wantLines := []string{
		`{"topic":"Tick","event_id":1,"created_at":1700000000000000001,"data":{"SymbolID":1,"Timestamp":1700000000000000000,"Side":0,"Price":50000.5,"Qty":1.25}}`,
		`{"topic":"OrderNew","event_id":2,"created_at":1700000000000000002,"data":{"AccountID":9,"ClientOrderID":77,"OrderID":-1,"SymbolID":1,"Side":0,"OrderType":0,"TimeInForce":0,"Quantity":2,"Price":49000,"ExecutedQty":0,"CreatedAt":0,"UpdatedAt":0}}`,
		`{"topic":"Timer","event_id":3,"created_at":1700000000000000003,"data":null}`,
		`{"topic":"Tick","event_id":4,"created_at":1700000000000000004,"data":{"decode_error":"buffer too small"}}`,
	}

	sc := bufio.NewScanner(bytes.NewReader(data))
	var got []string
	for sc.Scan() {
		got = append(got, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d lines, want %d\n%s", len(got), len(wantLines), data)
	}
	for i := range wantLines {
		if got[i] != wantLines[i] {
			t.Errorf("line %d mismatch:\n--- got ---\n%s\n--- want ---\n%s", i, got[i], wantLines[i])
		}
		// Also ensure each line is valid JSON.
		var rec jsonRecord
		if err := json.Unmarshal([]byte(got[i]), &rec); err != nil {
			t.Errorf("line %d not valid JSON: %v", i, err)
		}
	}
}

func TestMsgLog_WriteCommandJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewMsgLogger(dir)
	if err != nil {
		t.Fatalf("NewMsgLogger: %v", err)
	}

	submit := command.SubmitOrder{AccountID: 1, ClientOrderID: 42, SymbolID: 3, Quantity: 1.5, Price: 100}
	buf := make([]byte, submit.GetBufferLength())
	if err := submit.Encode(buf); err != nil {
		t.Fatal(err)
	}
	logger.LogCommand(Command{
		Ref:       CommandRef{CommandType: command.CommandTypeOrderSubmit, Length: uint64(len(buf))},
		CommandID: 9,
		CreatedAt: 100,
	}, buf)
	logger.Close()

	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "command_"+date+".jsonl"))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}
	var rec jsonRecord
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if rec.CommandType != "OrderSubmit" {
		t.Errorf("command_type = %q, want OrderSubmit", rec.CommandType)
	}
	if rec.CommandID != 9 {
		t.Errorf("command_id = %d, want 9", rec.CommandID)
	}
	if rec.CreatedAt != 100 {
		t.Errorf("created_at = %d, want 100", rec.CreatedAt)
	}
	if rec.Data == nil {
		t.Fatal("expected decoded data")
	}
}

// TestMsgLog_AppendsToExistingFile verifies a same-day restart appends
// additional JSONL lines without truncating prior records.
func TestMsgLog_AppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	payload := make([]byte, 0)
	ev := Event{Ref: EventRef{Topic: event.TopicEventTimer}, EventID: 1, CreatedAt: 1}

	l1, _ := NewMsgLogger(dir)
	l1.LogEvent(ev, payload)
	l1.Close()

	ev.EventID = 2
	ev.CreatedAt = 2
	l2, _ := NewMsgLogger(dir)
	l2.LogEvent(ev, payload)
	l2.Close()

	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "event_"+date+".jsonl"))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		n++
	}
	if n != 2 {
		t.Fatalf("got %d lines, want 2", n)
	}
}
