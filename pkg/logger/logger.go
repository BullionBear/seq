package logger

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
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

var (
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

	// Initialize file logger (lazy initialization on first use)
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
// Deprecated: Use GetLogger(LoggerTypeConsole) or GetLogger(LoggerTypeFile) instead
var Log = consoleLogger
