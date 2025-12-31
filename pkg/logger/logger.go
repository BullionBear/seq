package logger

import (
	"os"

	"github.com/rs/zerolog"
)

// Package-level variable that holds our configured logger instance.
// It starts with a disabled logger to be safe until it's initialized.
var Log zerolog.Logger = zerolog.New(nil).Level(zerolog.Disabled)

// InitLogger initializes the global logger with the desired configuration.
// This function should be called once, from main().
func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro // Use microsecond precision
	zerolog.SetGlobalLevel(zerolog.InfoLevel)             // Set default global level

	// Human-friendly output for local development
	outputWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05.000000", // Date and microsecond precision
	}
	zerolog.SetGlobalLevel(zerolog.DebugLevel) // More verbose in dev

	// For development, use the console writer
	Log = zerolog.New(outputWriter).
		With().
		Timestamp().
		Caller().
		Logger()
}
