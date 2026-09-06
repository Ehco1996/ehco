package ms

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/nakabonne/tstorage"
)

// DBHealth is the storage + latency snapshot the Settings page polls.
type DBHealth struct {
	FileBytes       int64                      `json:"db_file_bytes"`
	PageCount       int64                      `json:"db_page_count"`
	PageSize        int64                      `json:"db_page_size"`
	FreelistPages   int64                      `json:"db_freelist_pages"`
	NodeMetricsRows int64                      `json:"node_metrics_rows"`
	Stats           map[string]OpStatsSnapshot `json:"stats"`
}

func (ms *MetricsStore) Health(ctx context.Context) (*DBHealth, error) {
	h := &DBHealth{
		FileBytes:       ms.dirFileSize(),
		PageCount:       0,
		PageSize:        0,
		FreelistPages:   0,
		NodeMetricsRows: ms.nodeRows.Load(),
		Stats:           ms.stats.Snapshot(),
	}
	return h, nil
}

// MaintenanceResult is the common shape returned by maintenance ops.
type MaintenanceResult struct {
	NodeDeleted int64 `json:"node_deleted,omitempty"`
	BytesBefore int64 `json:"bytes_before,omitempty"`
	BytesAfter  int64 `json:"bytes_after,omitempty"`
	DurationMs  int64 `json:"duration_ms"`
}

// CleanupOlderThan triggers retention cleanup. In tstorage, retention is
// handled automatically at the partition level.
func (ms *MetricsStore) CleanupOlderThan(ctx context.Context, days int) (*MaintenanceResult, error) {
	defer track(&ms.stats.Cleanup)()
	start := time.Now()
	return &MaintenanceResult{
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// Vacuum reclaims storage. In TSDB there are no freelist pages or B-tree fragmentation.
func (ms *MetricsStore) Vacuum(ctx context.Context) (*MaintenanceResult, error) {
	defer track(&ms.stats.Vacuum)()
	start := time.Now()
	size := ms.dirFileSize()
	return &MaintenanceResult{
		BytesBefore: size,
		BytesAfter:  size,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

var ErrTruncateNotConfirmed = errors.New("truncate requires confirm=\"yes I am sure\"")

const truncateConfirm = "yes I am sure"

// Truncate empties all partitions.
func (ms *MetricsStore) Truncate(ctx context.Context, confirm string) (*MaintenanceResult, error) {
	if confirm != truncateConfirm {
		return nil, ErrTruncateNotConfirmed
	}
	defer track(&ms.stats.Truncate)()
	start := time.Now()
	before := ms.dirFileSize()
	nodeBefore := ms.nodeRows.Load()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.storage != nil {
		_ = ms.storage.Close()
	}
	_ = os.RemoveAll(ms.dirPath)
	_ = os.MkdirAll(ms.dirPath, 0o755)

	storage, err := tstorage.NewStorage(
		tstorage.WithDataPath(ms.dirPath),
		tstorage.WithPartitionDuration(1*time.Hour),
		tstorage.WithRetention(defaultRetentionDays*24*time.Hour),
		tstorage.WithTimestampPrecision(tstorage.Seconds),
	)
	if err != nil {
		return nil, err
	}
	ms.storage = storage
	ms.nodeRows.Store(0)

	after := ms.dirFileSize()
	ms.l.Warnf("truncate: deleted node=%d, %d -> %d bytes", nodeBefore, before, after)
	return &MaintenanceResult{
		NodeDeleted: nodeBefore,
		BytesBefore: before,
		BytesAfter:  after,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

func (ms *MetricsStore) ResetStats() {
	ms.stats.Reset()
}
