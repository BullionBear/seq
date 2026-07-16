package msgbus

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BullionBear/seq/core/model/event"
)

func TestMsgLog_FileHeaderRoundTrip(t *testing.T) {
	hdr := EncodeFileHeader(StreamTypeEvent)
	decoded, err := DecodeFileHeader(hdr[:])
	if err != nil {
		t.Fatalf("decode of freshly encoded header failed: %v", err)
	}
	if decoded.SchemaVersion != SchemaVersion {
		t.Errorf("schema version mismatch: %d != %d", decoded.SchemaVersion, SchemaVersion)
	}
	if decoded.StreamType != StreamTypeEvent {
		t.Errorf("stream type mismatch: %d != %d", decoded.StreamType, StreamTypeEvent)
	}
}

func TestMsgLog_RejectsBadHeaders(t *testing.T) {
	good := EncodeFileHeader(StreamTypeEvent)

	bad := good
	copy(bad[0:4], "XXXX")
	if _, err := DecodeFileHeader(bad[:]); err == nil {
		t.Error("expected magic rejection")
	}

	bad = good
	binary.LittleEndian.PutUint16(bad[4:6], 0xFFFE) // byte-swapped marker
	if _, err := DecodeFileHeader(bad[:]); err == nil {
		t.Error("expected endianness rejection")
	}

	bad = good
	binary.LittleEndian.PutUint16(bad[6:8], SchemaVersion+1)
	if _, err := DecodeFileHeader(bad[:]); err == nil {
		t.Error("expected schema version rejection")
	}

	bad = good
	bad[8] = 99
	if _, err := DecodeFileHeader(bad[:]); err == nil {
		t.Error("expected stream type rejection")
	}

	if _, err := DecodeFileHeader(good[:10]); err == nil {
		t.Error("expected truncation rejection")
	}
}

func TestMsgLog_WriteAndReadBack(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewMsgLogger(dir)
	if err != nil {
		t.Fatalf("NewMsgLogger: %v", err)
	}

	tick := event.Tick{SymbolID: 3, Price: 100.5, Qty: 2, Timestamp: 42}
	payload := make([]byte, tick.GetBufferLength())
	if err := tick.Encode(payload); err != nil {
		t.Fatalf("encode: %v", err)
	}
	ev := Event{
		Ref:       EventRef{Topic: event.TopicEventTick, Length: uint64(len(payload))},
		EventID:   7,
		CreatedAt: 123456789,
	}
	logger.LogEvent(ev, payload)
	logger.Close()

	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "event_"+date+".dat"))
	if err != nil {
		t.Fatalf("read .dat: %v", err)
	}
	if len(data) != FileHeaderSize+RecordHeaderSize+len(payload) {
		t.Fatalf("unexpected file size %d", len(data))
	}

	hdr, err := DecodeFileHeader(data[:FileHeaderSize])
	if err != nil {
		t.Fatalf("file header: %v", err)
	}
	if hdr.StreamType != StreamTypeEvent {
		t.Errorf("stream type = %d, want event", hdr.StreamType)
	}

	rec := data[FileHeaderSize:]
	if v := binary.LittleEndian.Uint16(rec[0:]); v != SchemaVersion {
		t.Errorf("record schema version = %d, want %d", v, SchemaVersion)
	}
	if mt := binary.LittleEndian.Uint16(rec[2:]); mt != uint16(event.TopicEventTick) {
		t.Errorf("msgType = %d, want %d", mt, uint16(event.TopicEventTick))
	}
	if l := binary.LittleEndian.Uint32(rec[4:]); int(l) != len(payload) {
		t.Errorf("length = %d, want %d", l, len(payload))
	}
	if id := binary.LittleEndian.Uint64(rec[8:]); id != 7 {
		t.Errorf("seqID = %d, want 7", id)
	}
	if ts := binary.LittleEndian.Uint64(rec[16:]); ts != 123456789 {
		t.Errorf("createdAt = %d, want 123456789", ts)
	}

	got, err := event.NewTickFromBytes(rec[RecordHeaderSize:])
	if err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if got != tick {
		t.Errorf("payload mismatch: %+v != %+v", got, tick)
	}
}

// TestMsgLog_AppendsToCompatibleFile verifies a same-day restart appends
// without writing a second file header.
func TestMsgLog_AppendsToCompatibleFile(t *testing.T) {
	dir := t.TempDir()
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	ev := Event{Ref: EventRef{Topic: event.TopicEventTick, Length: 8}, EventID: 1}

	l1, _ := NewMsgLogger(dir)
	l1.LogEvent(ev, payload)
	l1.Close()

	l2, _ := NewMsgLogger(dir)
	l2.LogEvent(ev, payload)
	l2.Close()

	date := time.Now().UTC().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "event_"+date+".dat"))
	if err != nil {
		t.Fatalf("read .dat: %v", err)
	}
	want := FileHeaderSize + 2*(RecordHeaderSize+len(payload))
	if len(data) != want {
		t.Fatalf("file size %d, want %d (single header, two records)", len(data), want)
	}
}

// TestMsgLog_MovesIncompatibleFileAside verifies an unversioned/foreign file
// is preserved under a suffix instead of being appended to or overwritten.
func TestMsgLog_MovesIncompatibleFileAside(t *testing.T) {
	dir := t.TempDir()
	date := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(dir, "event_"+date+".dat")
	legacy := []byte("legacy unversioned content")
	if err := os.WriteFile(path, legacy, 0644); err != nil {
		t.Fatal(err)
	}

	l, _ := NewMsgLogger(dir)
	l.LogEvent(Event{Ref: EventRef{Topic: event.TopicEventTick, Length: 8}}, make([]byte, 8))
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .dat: %v", err)
	}
	if _, err := DecodeFileHeader(data[:FileHeaderSize]); err != nil {
		t.Errorf("fresh file has invalid header: %v", err)
	}

	matches, _ := filepath.Glob(path + ".incompatible-*")
	if len(matches) != 1 {
		t.Fatalf("expected 1 preserved legacy file, found %d", len(matches))
	}
	preserved, _ := os.ReadFile(matches[0])
	if string(preserved) != string(legacy) {
		t.Error("legacy file content was not preserved")
	}
}
