package metrics

import (
	"fmt"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/node_exporter/collector"
)

func RegisterNodeExporterMetrics(cfg *config.Config) error {
	level := &promlog.AllowedLevel{}
	// mute node_exporter logger
	if err := level.Set("error"); err != nil {
		return err
	}

	logger := promlog.New(&promlog.Config{Level: level})
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %s", err)
	}
	prometheus.MustRegister(nc)
	return nil
}
