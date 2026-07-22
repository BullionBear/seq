package msgbus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/logger/rotate"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

func newTestMsgLogger(t *testing.T, dir string) *MsgLogger {
	t.Helper()
	l, err := NewMsgLogger(rotate.Policy{
		Dir: dir, BaseName: "msg", Ext: "jsonl",
		Daily: true, Sync: rotate.SyncNone,
	})
	if err != nil {
		t.Fatalf("NewMsgLogger: %v", err)
	}
	return l
}

func msgFile(dir string) string {
	date := time.Now().UTC().Format("2006-01-02")
	return filepath.Join(dir, "msg_"+date+".jsonl")
}

func TestMsgLog_WriteJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := newTestMsgLogger(t, dir)

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

	logger.LogEvent(Event{
		Ref:       EventRef{Topic: event.TopicEventTick, Length: 4},
		EventID:   4,
		CreatedAt: 1700000000000000004,
	}, tickBuf[:4])

	logger.Close()

	data, err := os.ReadFile(msgFile(dir))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}

	wantLines := []string{
		`{"kind":"event","topic":"Tick","event_id":1,"created_at":1700000000000000001,"data":{"SymbolID":1,"Timestamp":1700000000000000000,"Side":0,"Price":50000.5,"Qty":1.25}}`,
		`{"kind":"event","topic":"OrderNew","event_id":2,"created_at":1700000000000000002,"data":{"AccountID":9,"ClientOrderID":77,"OrderID":-1,"SymbolID":1,"Side":0,"OrderType":0,"TimeInForce":0,"Quantity":2,"Price":49000,"ExecutedQty":0,"CreatedAt":0,"UpdatedAt":0}}`,
		`{"kind":"event","topic":"Timer","event_id":3,"created_at":1700000000000000003,"data":null}`,
		`{"kind":"event","topic":"Tick","event_id":4,"created_at":1700000000000000004,"data":{"decode_error":"buffer too small"}}`,
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
		var rec map[string]any
		if err := json.Unmarshal([]byte(got[i]), &rec); err != nil {
			t.Errorf("line %d not valid JSON: %v", i, err)
		}
		if rec["kind"] != "event" {
			t.Errorf("line %d kind=%v", i, rec["kind"])
		}
	}
}

func TestMsgLog_WriteCommandJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := newTestMsgLogger(t, dir)

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

	data, err := os.ReadFile(msgFile(dir))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if rec["kind"] != "command" {
		t.Errorf("kind = %v, want command", rec["kind"])
	}
	if rec["command_type"] != "OrderSubmit" {
		t.Errorf("command_type = %v, want OrderSubmit", rec["command_type"])
	}
	if rec["command_id"].(float64) != 9 {
		t.Errorf("command_id = %v, want 9", rec["command_id"])
	}
}

func TestMsgLog_MergedEventAndCommand(t *testing.T) {
	dir := t.TempDir()
	logger := newTestMsgLogger(t, dir)

	logger.LogEvent(Event{Ref: EventRef{Topic: event.TopicEventTimer}, EventID: 1, CreatedAt: 1}, nil)
	submit := command.CancelAll{AccountID: 2, SymbolID: 3}
	buf := make([]byte, submit.GetBufferLength())
	_ = submit.Encode(buf)
	logger.LogCommand(Command{
		Ref:       CommandRef{CommandType: command.CommandTypeCancelAll, Length: uint64(len(buf))},
		CommandID: 2,
		CreatedAt: 2,
	}, buf)
	logger.Close()

	data, err := os.ReadFile(msgFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines in single file, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"kind":"event"`) {
		t.Fatalf("line0=%s", lines[0])
	}
	if !strings.Contains(lines[1], `"kind":"command"`) {
		t.Fatalf("line1=%s", lines[1])
	}
}

func TestMsgLog_AppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	payload := make([]byte, 0)
	ev := Event{Ref: EventRef{Topic: event.TopicEventTimer}, EventID: 1, CreatedAt: 1}

	l1 := newTestMsgLogger(t, dir)
	l1.LogEvent(ev, payload)
	l1.Close()

	ev.EventID = 2
	ev.CreatedAt = 2
	l2 := newTestMsgLogger(t, dir)
	l2.LogEvent(ev, payload)
	l2.Close()

	data, err := os.ReadFile(msgFile(dir))
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

func BenchmarkMsgLogger_LogEvent(b *testing.B) {
	dir := b.TempDir()
	l, err := NewMsgLogger(rotate.Policy{
		Dir: dir, BaseName: "msg", Ext: "jsonl",
		Daily: false, Sync: rotate.SyncNone,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()

	tick := event.Tick{SymbolID: 1, Price: 50000.5, Qty: 1.25, Timestamp: 1700000000000000000}
	tickBuf := make([]byte, tick.GetBufferLength())
	if err := tick.Encode(tickBuf); err != nil {
		b.Fatal(err)
	}
	ev := Event{
		Ref:       EventRef{Topic: event.TopicEventTick, Length: uint64(len(tickBuf))},
		EventID:   1,
		CreatedAt: 1,
	}
	// Warm up.
	l.LogEvent(ev, tickBuf)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.LogEvent(ev, tickBuf)
	}
}
