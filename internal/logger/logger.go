package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// SetupLogger initializes the structured logger
func SetupLogger(logLevel, logFormat string) *slog.Logger {
	var handler slog.Handler
	var output io.Writer = os.Stdout

	level := slogLevelFromString(logLevel)

	if strings.ToLower(logFormat) == "json" {
		handler = slog.NewJSONHandler(output, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(output, &slog.HandlerOptions{
			Level: level,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// slogLevelFromString converts a string to slog.Level
func slogLevelFromString(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
