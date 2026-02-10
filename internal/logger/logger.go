// Package logger provides structured logging configuration for the application.
// It configures log/slog with JSON output format and source location tracking,
// making logs machine-parseable and suitable for log aggregation systems.
package logger

import (
	"log/slog"
	"os"
)

// Setup initializes the global slog logger with JSON output and source location.
// Source location tracking helps identify exactly where log entries originated.
func Setup(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})
	slog.SetDefault(slog.New(handler))
}

// ParseLevel converts a string log level to slog.Level.
// Valid values: "debug", "info", "warn", "error".
// Unrecognized values default to info level.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
