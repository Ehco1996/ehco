package metrics

import (
	"log/slog"
	"strings"
)

func zapLevelToSlogLevel(zapLevel string) slog.Level {
	switch strings.ToLower(zapLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
