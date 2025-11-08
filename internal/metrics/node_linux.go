//go:build linux

package metrics

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/node_exporter/collector"
)

// zapLevelToSlogLevel converts zap log level string to slog.Level
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

func RegisterNodeExporterMetrics(cfg *config.Config) error {
	slogLevel := zapLevelToSlogLevel(cfg.LogLeveL)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	}))

	// node_exporter relay on `kingpin` to enable default node collector
	// see https://github.com/prometheus/node_exporter/pull/2463
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		return err
	}
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %s", err)
	}
	prometheus.MustRegister(nc)
	return nil
}
