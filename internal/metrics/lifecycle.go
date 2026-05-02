package metrics

import (
	"context"

	"github.com/Ehco1996/ehco/internal/config"
	"go.uber.org/zap"
)

// Start launches background collectors. Call once per process.
func Start(ctx context.Context, cfg *config.Config) {
	l := zap.S().Named("metrics")
	go startNodeCollector(ctx, l)
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		go pg.Run()
	}
}
