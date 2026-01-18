package logger

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LoggerType represents the type of logger to retrieve
type LoggerType int

const (
	// LoggerTypeConsole represents console/terminal output logger
	LoggerTypeConsole LoggerType = iota
	// LoggerTypeFile represents file output logger
	LoggerTypeFile
)

const (
	// DefaultLogFile is the default log file path
	DefaultLogFile = "logs/seq.log"
	// EnvLogFile is the environment variable name for log file path
	EnvLogFile = "SEQ_LOG_FILE"
)

// Options contains configuration options for the logger
type Options struct {
	Level          string // Log level: trace, debug, info, warn, error, fatal, panic
	Output         string // Output type: "stdout" or "file"
	Path           string // Log file path (required when Output is "file")
	MaxByteSize    int    // Max size in bytes before rotation (0 = no rotation)
	MaxBackupFiles int    // Max number of backup files to keep (0 = keep all)
}

var (
	// globalLogger is the singleton logger instance
	globalLogger zerolog.Logger
	// globalLoggerInitialized tracks if the global logger has been initialized
	globalLoggerInitialized bool
	// globalLoggerMutex protects global logger initialization
	globalLoggerMutex sync.RWMutex

	// consoleLogger is initialized for console/terminal output
	consoleLogger zerolog.Logger
	// fileLogger is initialized for file output
	fileLogger zerolog.Logger
	// logFile stores the current log file path
	logFile string
	// fileLoggerInit ensures file logger is initialized only once
	fileLoggerInit sync.Once
	// fileLoggerMutex protects file logger reinitialization
	fileLoggerMutex sync.RWMutex
	// fileLoggerInitialized tracks if file logger has been initialized
	fileLoggerInitialized bool
	// consoleWriter is a package-level variable to reduce escapes
	// Using a pointer to avoid copying the struct when passing to zerolog.New
	consoleWriter = &zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05.000000", // Date and microsecond precision
	}
)

// Init initializes both console and file loggers
func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro // Use microsecond precision
	zerolog.SetGlobalLevel(zerolog.DebugLevel)            // More verbose in dev

	// Initialize console logger with human-friendly output
	// Using the package-level consoleWriter pointer to reduce heap allocations
	consoleLogger = zerolog.New(consoleWriter).
		With().
		Timestamp().
		Caller().
		Logger()

	// Determine log file path: environment variable > SetLogFile() > default
	logFile = os.Getenv(EnvLogFile)
	if logFile == "" {
		logFile = DefaultLogFile
	}

	// Initialize global logger to console by default
	globalLogger = consoleLogger
}

// parseLogLevel parses a string log level and returns the corresponding zerolog.Level
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
		return zerolog.DebugLevel // Default to debug
	}
}

// createFileWriter creates a file writer with optional log rotation
func createFileWriter(path string, maxByteSize int, maxBackupFiles int) (io.Writer, error) {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	// If no rotation is configured, use a simple file writer
	if maxByteSize <= 0 {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		return file, nil
	}

	// Use lumberjack for log rotation
	// Convert bytes to megabytes for lumberjack (it expects MB)
	maxSizeMB := maxByteSize / (1024 * 1024)
	if maxSizeMB <= 0 {
		maxSizeMB = 1 // Minimum 1 MB
	}

	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSizeMB,      // Maximum size in megabytes before rotation
		MaxBackups: maxBackupFiles, // Maximum number of old log files to retain
		MaxAge:     0,              // Don't delete by age, only by count
		Compress:   false,          // Don't compress by default
	}, nil
}

// Init initializes the global singleton logger with the provided options
// This should be called once at application startup
func Init(opts Options) error {
	globalLoggerMutex.Lock()
	defer globalLoggerMutex.Unlock()

	// Set log level
	level := parseLogLevel(opts.Level)
	zerolog.SetGlobalLevel(level)

	// Determine output type (default to stdout if not specified)
	output := strings.ToLower(opts.Output)
	if output == "" {
		output = "stdout"
	}

	var writer io.Writer

	switch output {
	case "stdout", "console":
		// Use console writer for stdout
		writer = consoleWriter
	case "file":
		// Validate that path is provided
		path := opts.Path
		if path == "" {
			// Fall back to default path or environment variable
			path = os.Getenv(EnvLogFile)
			if path == "" {
				path = DefaultLogFile
			}
		}

		// Create file writer with rotation support
		fileWriter, err := createFileWriter(path, opts.MaxByteSize, opts.MaxBackupFiles)
		if err != nil {
			return err
		}

		writer = fileWriter
		logFile = path

		// Also update the file logger for backward compatibility
		fileLoggerMutex.Lock()
		fileLogger = zerolog.New(writer).
			With().
			Timestamp().
			Caller().
			Logger().
			Level(level)
		fileLoggerInitialized = true
		fileLoggerMutex.Unlock()
	default:
		// Unknown output type, default to stdout
		writer = consoleWriter
	}

	// Create global logger with the selected writer
	globalLogger = zerolog.New(writer).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(level)

	globalLoggerInitialized = true
	return nil
}

// Get returns the global singleton logger instance
// If the logger hasn't been initialized with Init(), returns the default console logger
func Get() zerolog.Logger {
	globalLoggerMutex.RLock()
	defer globalLoggerMutex.RUnlock()

	if globalLoggerInitialized {
		return globalLogger
	}

	// Fall back to console logger if not initialized
	return consoleLogger
}

// initFileLogger initializes the file logger (called lazily on first access)
func initFileLogger() {
	fileLoggerMutex.Lock()
	defer fileLoggerMutex.Unlock()

	// Create logs directory if it doesn't exist
	// Inline directory creation to reduce variable escapes
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		// If we can't create the directory, fall back to console logger
		fileLogger = consoleLogger
		fileLoggerInitialized = true
		return
	}

	// Open or create log file (append mode)
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// If we can't open the file, fall back to console logger
		fileLogger = consoleLogger
		fileLoggerInitialized = true
		return
	}

	fileLogger = zerolog.New(file).
		With().
		Timestamp().
		Caller().
		Logger()
	fileLoggerInitialized = true
}

// getFileLogger returns the file logger, initializing it if necessary
func getFileLogger() zerolog.Logger {
	fileLoggerInit.Do(initFileLogger)
	fileLoggerMutex.RLock()
	defer fileLoggerMutex.RUnlock()
	return fileLogger
}

// SetLogFile sets the log file path for the file logger.
// This can be called at any time:
// - Before first use: Sets the path for initial initialization
// - After first use: Reinitializes the file logger with the new path
func SetLogFile(path string) error {
	fileLoggerMutex.Lock()
	defer fileLoggerMutex.Unlock()

	logFile = path

	// Reinitialize file logger if it was already initialized
	if fileLoggerInitialized {
		// Inline directory creation to reduce variable escapes
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
			return err
		}

		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}

		fileLogger = zerolog.New(file).
			With().
			Timestamp().
			Caller().
			Logger()
	}

	return nil
}

// GetLogFile returns the current log file path
func GetLogFile() string {
	fileLoggerMutex.RLock()
	defer fileLoggerMutex.RUnlock()
	if logFile == "" {
		return DefaultLogFile
	}
	return logFile
}

// GetLogger returns the logger instance based on the LoggerType
// This maintains backward compatibility with existing code
func GetLogger(loggerType LoggerType) zerolog.Logger {
	switch loggerType {
	case LoggerTypeFile:
		return getFileLogger()
	case LoggerTypeConsole:
		return consoleLogger
	default:
		// Default to console logger for unknown types
		return consoleLogger
	}
}

// Log is kept for backward compatibility, defaults to console logger
// Deprecated: Use Get() for the singleton logger, or GetLogger(LoggerTypeConsole)/GetLogger(LoggerTypeFile) for specific loggers
var Log = consoleLogger
