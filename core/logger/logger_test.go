package logger

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/BullionBear/seq/core/logger/rotate"
	"github.com/rs/zerolog"
)

func TestLogger(t *testing.T) {
	Log.Info().Msg("Hello, World!")
}

func TestLogger_WithFields(t *testing.T) {
	Log.Info().Str("name", "John").Int("age", 30).Msg("Hello, World!")
}

func TestInit_FileAndStdout(t *testing.T) {
	dir := t.TempDir()
	stdout := false
	err := Init(Options{
		Level:  "info",
		Stdout: false,
		File: rotate.FileConfig{
			Dir:      dir,
			Name:     "seq",
			MaxBytes: 0,
			Daily:    true,
			Sync:     rotate.SyncNone,
		},
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()
	_ = stdout

	log := Get()
	log.Info().Str("test", "init").Msg("file logger")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected log file to be created")
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no .log file in %s: %v", dir, entries)
	}
}

// BenchmarkLogger_Info benchmarks the basic Info logging operation
func BenchmarkLogger_Info(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log.Info().Msg("Hello, World!")
	}
}

// BenchmarkLogger_WithFields benchmarks logging with structured fields
func BenchmarkLogger_WithFields(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log.Info().Str("name", "John").Int("age", 30).Msg("Hello, World!")
	}
}

// BenchmarkLogger_ZeroAlloc tests if logging is zero-allocation when using io.Discard
func BenchmarkLogger_ZeroAlloc(b *testing.B) {
	zeroLog := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		zeroLog.Info().Msg("Hello, World!")
	}
}

// TestLogger_ZeroAllocation verifies that logging with a discard writer has zero allocations
func TestLogger_ZeroAllocation(t *testing.T) {
	zeroLog := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	allocs := testing.AllocsPerRun(1000, func() {
		zeroLog.Info().Str("key", "value").Int("num", 42).Msg("test message")
	})
	if allocs > 0 {
		t.Errorf("Expected zero allocations, got %d", int(allocs))
	}
}
