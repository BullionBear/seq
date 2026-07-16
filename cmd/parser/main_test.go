package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

// genGoldenDat builds a deterministic event .dat fixture: valid header,
// a Tick record, an OrderNew record, an empty-payload record, and a record
// whose payload is truncated (must surface as an inline decode_error).
func genGoldenDat(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer

	var hdr [msgbus.FileHeaderSize]byte
	copy(hdr[0:4], msgbus.MagicBytes[:])
	binary.LittleEndian.PutUint16(hdr[4:6], msgbus.EndiannessMarker)
	binary.LittleEndian.PutUint16(hdr[6:8], msgbus.SchemaVersion)
	hdr[8] = msgbus.StreamTypeEvent
	copy(hdr[16:], "golden-test")
	buf.Write(hdr[:])

	writeRec := func(topic event.Topic, seqID, createdAt uint64, payload []byte) {
		var rh [msgbus.RecordHeaderSize]byte
		binary.LittleEndian.PutUint16(rh[0:], msgbus.SchemaVersion)
		binary.LittleEndian.PutUint16(rh[2:], uint16(topic))
		binary.LittleEndian.PutUint32(rh[4:], uint32(len(payload)))
		binary.LittleEndian.PutUint64(rh[8:], seqID)
		binary.LittleEndian.PutUint64(rh[16:], createdAt)
		buf.Write(rh[:])
		buf.Write(payload)
	}

	tick := event.Tick{SymbolID: 1, Price: 50000.5, Qty: 1.25, Timestamp: 1700000000000000000}
	tickBuf := make([]byte, tick.GetBufferLength())
	if err := tick.Encode(tickBuf); err != nil {
		t.Fatal(err)
	}
	writeRec(event.TopicEventTick, 1, 1700000000000000001, tickBuf)

	orderNew := event.OrderNew{AccountID: 9, ClientOrderID: 77, OrderID: -1, SymbolID: 1, Quantity: 2, Price: 49000}
	orderBuf := make([]byte, orderNew.GetBufferLength())
	if err := orderNew.Encode(orderBuf); err != nil {
		t.Fatal(err)
	}
	writeRec(event.TopicEventOrderNew, 2, 1700000000000000002, orderBuf)

	writeRec(event.TopicEventTimer, 3, 1700000000000000003, nil)

	// Truncated Tick payload: decoder must reject, parser must report inline.
	writeRec(event.TopicEventTick, 4, 1700000000000000004, tickBuf[:4])

	return buf.Bytes()
}

func TestParseGoldenFile(t *testing.T) {
	datPath := filepath.Join("testdata", "event_golden.dat")
	jsonlPath := filepath.Join("testdata", "event_golden.jsonl")

	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		dat := genGoldenDat(t)
		if err := os.WriteFile(datPath, dat, 0644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parseFile(bytes.NewReader(dat), &out); err != nil {
			t.Fatalf("parse of regenerated fixture failed: %v", err)
		}
		if err := os.WriteFile(jsonlPath, out.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dat, err := os.ReadFile(datPath)
	if err != nil {
		t.Fatalf("read fixture (run with -update to generate): %v", err)
	}
	want, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read fixture (run with -update to generate): %v", err)
	}

	var out bytes.Buffer
	if err := parseFile(bytes.NewReader(dat), &out); err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Errorf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.Bytes(), want)
	}
}

// TestParseRejectsUnversionedFile: pre-P0-4 files (no file header) must be
// refused with a magic error, not misparsed.
func TestParseRejectsUnversionedFile(t *testing.T) {
	// Old format: [topic(4)][seqID(8)][createdAt(8)][len(4)][payload].
	var old bytes.Buffer
	var rh [24]byte
	binary.LittleEndian.PutUint32(rh[0:], uint32(event.TopicEventTick))
	binary.LittleEndian.PutUint64(rh[4:], 1)
	binary.LittleEndian.PutUint64(rh[12:], 2)
	binary.LittleEndian.PutUint32(rh[20:], 0)
	old.Write(rh[:])
	old.Write(make([]byte, 64)) // some payload bytes

	err := parseFile(bytes.NewReader(old.Bytes()), &bytes.Buffer{})
	if !errors.Is(err, msgbus.ErrBadMagic) {
		t.Errorf("expected ErrBadMagic, got %v", err)
	}
}

// TestParseRejectsFutureSchema: files written by a newer schema must be refused.
func TestParseRejectsFutureSchema(t *testing.T) {
	dat := genGoldenDat(t)
	binary.LittleEndian.PutUint16(dat[6:8], msgbus.SchemaVersion+1)
	err := parseFile(bytes.NewReader(dat), &bytes.Buffer{})
	if !errors.Is(err, msgbus.ErrUnsupportedSchema) {
		t.Errorf("expected ErrUnsupportedSchema, got %v", err)
	}
}

// TestParseRejectsFutureRecordSchema: a record stamped with a newer schema
// (e.g. mixed-version file after a partial upgrade) must be refused.
func TestParseRejectsFutureRecordSchema(t *testing.T) {
	dat := genGoldenDat(t)
	// First record header starts right after the file header.
	binary.LittleEndian.PutUint16(dat[msgbus.FileHeaderSize:], msgbus.SchemaVersion+1)
	err := parseFile(bytes.NewReader(dat), &bytes.Buffer{})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("unsupported record schema")) {
		t.Errorf("expected record schema rejection, got %v", err)
	}
}
