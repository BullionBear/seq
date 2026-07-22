package logger

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/BullionBear/seq/core/logger/rotate"
	"github.com/rs/zerolog"
)

// Options contains configuration options for the logger.
type Options struct {
	Level  string            // Log level: trace, debug, info, warn, error, fatal, panic
	Stdout bool              // Write human-readable logs to stdout (default true via Config)
	File   rotate.FileConfig // File output; empty Dir disables file logging
}

var (
	globalLogger            zerolog.Logger
	globalLoggerInitialized bool
	globalLoggerMutex       sync.RWMutex

	consoleLogger zerolog.Logger
	fileWriter    *rotate.Writer // retained so Close/Sync can reach it

	consoleWriter = &zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05.000000",
	}
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	consoleLogger = zerolog.New(consoleWriter).
		With().
		Timestamp().
		Caller().
		Logger()

	globalLogger = consoleLogger
}

func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.DebugLevel
	}
}

// Init initializes the global singleton logger with the provided options.
func Init(opts Options) error {
	globalLoggerMutex.Lock()
	defer globalLoggerMutex.Unlock()

	level := parseLogLevel(opts.Level)
	zerolog.SetGlobalLevel(level)

	var writers []io.Writer
	if opts.Stdout {
		writers = append(writers, consoleWriter)
	}

	if opts.File.Enabled() {
		pol := opts.File.ToPolicy("log")
		if pol.BaseName == "" {
			pol.BaseName = "seq"
		}
		w, err := rotate.NewWriter(pol)
		if err != nil {
			return err
		}
		if fileWriter != nil {
			_ = fileWriter.Close()
		}
		fileWriter = w
		writers = append(writers, w)
	}

	var writer io.Writer
	switch len(writers) {
	case 0:
		writer = io.Discard
	case 1:
		writer = writers[0]
	default:
		writer = io.MultiWriter(writers...)
	}

	globalLogger = zerolog.New(writer).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(level)

	globalLoggerInitialized = true
	return nil
}

// Get returns the global singleton logger instance.
func Get() zerolog.Logger {
	globalLoggerMutex.RLock()
	defer globalLoggerMutex.RUnlock()

	if globalLoggerInitialized {
		return globalLogger
	}
	return consoleLogger
}

// Sync fsyncs the file writer when SyncPeriodic (or any explicit sync) is desired.
func Sync() error {
	globalLoggerMutex.RLock()
	w := fileWriter
	globalLoggerMutex.RUnlock()
	if w == nil {
		return nil
	}
	return w.Sync()
}

// Close closes the underlying file writer, if any.
func Close() error {
	globalLoggerMutex.Lock()
	defer globalLoggerMutex.Unlock()
	if fileWriter == nil {
		return nil
	}
	err := fileWriter.Close()
	fileWriter = nil
	return err
}

// Log is kept for backward compatibility; defaults to console logger.
// Deprecated: Use Get() for the singleton logger.
var Log = consoleLogger
