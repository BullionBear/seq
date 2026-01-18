package logger

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogger(t *testing.T) {
	Log.Info().Msg("Hello, World!")
}

func TestLogger_WithFields(t *testing.T) {
	Log.Info().Str("name", "John").Int("age", 30).Msg("Hello, World!")
}

func TestLogger_WithLevel(t *testing.T) {
	Log.Debug().Msg("Hello, World!")
}

func TestLogger_Warn(t *testing.T) {
	Log.Warn().Msg("Hello, World!")
}

// TestGetLogger tests the GetLogger function with different logger types
func TestGetLogger(t *testing.T) {
	// Test console logger
	consoleLog := GetLogger(LoggerTypeConsole)
	consoleLog.Info().Msg("Console log message")

	// Test file logger
	fileLog := GetLogger(LoggerTypeFile)
	fileLog.Info().Msg("File log message")

	// Test default case (unknown type should return console logger)
	defaultLog := GetLogger(LoggerType(999))
	defaultLog.Info().Msg("Default log message")
}

// TestGetLogger_Console tests console logger specifically
func TestGetLogger_Console(t *testing.T) {
	log := GetLogger(LoggerTypeConsole)
	log.Info().Str("test", "console").Msg("Testing console logger")
}

// TestGetLogger_File tests file logger specifically
func TestGetLogger_File(t *testing.T) {
	log := GetLogger(LoggerTypeFile)
	log.Info().Str("test", "file").Msg("Testing file logger")
}

// TestSetLogFile tests changing the log file path
func TestSetLogFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testLogFile := filepath.Join(tmpDir, "test.log")

	// Set the log file path
	err := SetLogFile(testLogFile)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Verify the path was set
	currentPath := GetLogFile()
	if currentPath != testLogFile {
		t.Errorf("Expected log file path %s, got %s", testLogFile, currentPath)
	}

	// Use the file logger
	fileLog := GetLogger(LoggerTypeFile)
	fileLog.Info().Str("test", "setlogfile").Msg("Testing SetLogFile")

	// Verify the log file was created
	if _, err := os.Stat(testLogFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", testLogFile)
	}
}

// TestSetLogFile_AfterUse tests changing the log file path after logger has been used
func TestSetLogFile_AfterUse(t *testing.T) {
	// First use the file logger (this initializes it)
	fileLog := GetLogger(LoggerTypeFile)
	fileLog.Info().Msg("Initial log message")

	// Now change the log file path
	tmpDir := t.TempDir()
	newLogFile := filepath.Join(tmpDir, "new.log")

	err := SetLogFile(newLogFile)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Use the file logger again - should use new path
	fileLog2 := GetLogger(LoggerTypeFile)
	fileLog2.Info().Str("test", "afteruse").Msg("Testing SetLogFile after use")

	// Verify the new log file was created
	if _, err := os.Stat(newLogFile); os.IsNotExist(err) {
		t.Errorf("New log file was not created at %s", newLogFile)
	}
}

// TestGetLogFile tests getting the current log file path
func TestGetLogFile(t *testing.T) {
	path := GetLogFile()
	if path == "" {
		t.Error("GetLogFile returned empty string")
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

// BenchmarkLogger_WithLevel benchmarks different log levels
func BenchmarkLogger_Debug(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log.Debug().Msg("Hello, World!")
	}
}

// BenchmarkLogger_Warn benchmarks Warn level logging
func BenchmarkLogger_Warn(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log.Warn().Msg("Hello, World!")
	}
}

// BenchmarkLogger_ZeroAlloc tests if logging is zero-allocation when using io.Discard
// io.Discard is a special writer in Go's io package that discards all data written to it.
// This is the true zero-allocation scenario for zerolog - no I/O overhead, no formatting overhead.
func BenchmarkLogger_ZeroAlloc(b *testing.B) {
	// Create a logger with io.Discard (zero-allocation mode)
	// io.Discard is a Writer on which all Write calls succeed without doing anything.
	zeroLog := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		zeroLog.Info().Msg("Hello, World!")
	}
}

// BenchmarkLogger_FileOutput benchmarks logging to a file.
// Note: zerolog is highly optimized and uses internal buffer pools, so it may show
// 0 allocations per operation even when writing to files. However, file I/O still
// incurs significant performance overhead compared to io.Discard (see benchmark results).
// The allocations happen in the buffer pool management, which is amortized across operations.
func BenchmarkLogger_FileOutput(b *testing.B) {
	// Create a temporary file for testing
	f, err := os.CreateTemp("", "logger_benchmark_*.log")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	fileLog := zerolog.New(f).Level(zerolog.InfoLevel)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fileLog.Info().Msg("Hello, World!")
	}
}

// TestLogger_ZeroAllocation verifies that logging with a nil/discard writer has zero allocations
func TestLogger_ZeroAllocation(t *testing.T) {
	zeroLog := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	allocs := testing.AllocsPerRun(1000, func() {
		zeroLog.Info().Str("key", "value").Int("num", 42).Msg("test message")
	})
	if allocs > 0 {
		t.Errorf("Expected zero allocations, got %d", int(allocs))
	}
}

// TestLogger_FileAllocation demonstrates allocations when logging to a file
func TestLogger_FileAllocation(t *testing.T) {
	f, err := os.CreateTemp("", "logger_test_*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	fileLog := zerolog.New(f).Level(zerolog.InfoLevel)
	allocs := testing.AllocsPerRun(1000, func() {
		fileLog.Info().Str("key", "value").Int("num", 42).Msg("test message")
	})
	// File output typically allocates memory for buffer management, JSON encoding, I/O operations
	// Note: zerolog is highly optimized, so allocations may be minimal or zero in some cases
	t.Logf("File output allocations per operation: %.2f", allocs)
}
