//go:build darwin

package metrics

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/node_exporter/collector"
)

// `thermal` collector logs an ERROR per scrape on most Macs with
// "no CPU power status has been recorded". Disable it via kingpin so
// logs stay quiet. Kept separate from node_linux.go because Linux
// doesn't have the collector and shouldn't carry the flag noise.
func RegisterNodeExporterMetrics(cfg *config.Config) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: zapLevelToSlogLevel(cfg.LogLeveL),
	}))

	if _, err := kingpin.CommandLine.Parse([]string{
		"--no-collector.thermal",
	}); err != nil {
		return err
	}
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %w", err)
	}
	prometheus.MustRegister(nc)
	return nil
}
