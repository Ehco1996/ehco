//go:build darwin

package metrics

import (
	"github.com/Ehco1996/ehco/internal/config"
)

// RegisterNodeExporterMetrics is a no-op on Darwin/macOS
// node_exporter has compatibility issues on macOS, so we disable it
func RegisterNodeExporterMetrics(cfg *config.Config) error {
	// node_exporter is not supported on macOS, skip registration
	return nil
}
