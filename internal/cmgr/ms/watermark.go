package ms

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	retention           = 30 * 24 * time.Hour
	watermarkInterval   = 5 * time.Minute
	defaultWatermarkPct = 50
)

type DiskUsageProber func(path string) (usedPct float64, err error)

func defaultDiskUsage(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	if total == 0 {
		return 0, errors.New("zero-sized filesystem")
	}
	avail := stat.Bavail * uint64(stat.Bsize)
	used := total - avail
	return float64(used) / float64(total) * 100, nil
}

func (ms *MetricsStore) startWatermark(ctx context.Context, limitPct int) {
	if limitPct <= 0 {
		limitPct = defaultWatermarkPct
	}
	go ms.runWatermark(ctx, limitPct, defaultDiskUsage)
}

func (ms *MetricsStore) runWatermark(ctx context.Context, limitPct int, prober DiskUsageProber) {
	t := time.NewTicker(watermarkInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ms.evictUntilBelow(limitPct, prober)
		}
	}
}

func (ms *MetricsStore) evictUntilBelow(limitPct int, prober DiskUsageProber) {
	for {
		pct, err := prober(ms.dir)
		if err != nil {
			ms.logger().Warnf("watermark probe: %v", err)
			return
		}
		if pct < float64(limitPct) {
			return
		}
		victim, err := oldestPartition(ms.dir)
		if err != nil {
			ms.logger().Warnf("watermark scan: %v", err)
			return
		}
		if victim == "" {
			ms.logger().Warnf("watermark: disk %.1f%% but no partition to evict", pct)
			return
		}
		ms.logger().Warnf("watermark: disk %.1f%% > %d%%, evicting %s", pct, limitPct, victim)
		if err := os.RemoveAll(victim); err != nil {
			ms.logger().Errorf("watermark remove %s: %v", victim, err)
			return
		}
	}
}

func (ms *MetricsStore) logger() *zap.SugaredLogger {
	if ms.l != nil {
		return ms.l
	}
	return zap.NewNop().Sugar()
}

func oldestPartition(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type p struct {
		path  string
		mtime time.Time
	}
	parts := make([]p, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		parts = append(parts, p{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(parts) == 0 {
		return "", nil
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].mtime.Before(parts[j].mtime) })
	return parts[0].path, nil
}
