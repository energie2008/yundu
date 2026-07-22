package config

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(serviceName, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler).With("service", serviceName)
	slog.SetDefault(logger)
	return logger
}
