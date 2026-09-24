package store

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/nakabonne/tstorage"
)

// DBHealth is the storage + latency snapshot the Settings page polls.
type DBHealth struct {
	FileBytes         int64                      `json:"db_file_bytes"`
	Partitions        int                        `json:"partitions"`
	PartitionDuration string                     `json:"partition_duration"`
	RetentionDays     int                        `json:"retention_days"`
	NodeMetricsRows   int64                      `json:"node_metrics_rows"`
	Stats             map[string]OpStatsSnapshot `json:"stats"`
}

func (s *Store) Health(ctx context.Context) (*DBHealth, error) {
	partitions, _, totalBytes := s.countPartitionsAndFiles()
	h := &DBHealth{
		FileBytes:         totalBytes,
		Partitions:        partitions,
		PartitionDuration: "1h",
		RetentionDays:     defaultRetentionDays,
		NodeMetricsRows:   s.countSamples(),
		Stats:             s.stats.Snapshot(),
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
// handled automatically by background partition pruning.
func (s *Store) CleanupOlderThan(ctx context.Context, days int) (*MaintenanceResult, error) {
	defer track(&s.stats.Cleanup)()
	start := time.Now()
	before := s.dirFileSize()
	return &MaintenanceResult{
		BytesBefore: before,
		BytesAfter:  s.dirFileSize(),
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

var ErrTruncateNotConfirmed = errors.New("truncate requires confirm=\"yes I am sure\"")

const truncateConfirm = "yes I am sure"

// Truncate empties all partitions and reinitializes storage.
func (s *Store) Truncate(ctx context.Context, confirm string) (*MaintenanceResult, error) {
	if confirm != truncateConfirm {
		return nil, ErrTruncateNotConfirmed
	}
	defer track(&s.stats.Truncate)()
	start := time.Now()
	before := s.dirFileSize()
	nodeBefore := s.countSamples()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storage != nil {
		_ = s.storage.Close()
	}
	_ = os.RemoveAll(s.dirPath)
	_ = os.MkdirAll(s.dirPath, 0o755)

	storage, err := tstorage.NewStorage(
		tstorage.WithDataPath(s.dirPath),
		tstorage.WithPartitionDuration(1*time.Hour),
		tstorage.WithRetention(defaultRetentionDays*24*time.Hour),
		tstorage.WithTimestampPrecision(tstorage.Seconds),
	)
	if err != nil {
		return nil, err
	}
	s.storage = storage

	after := s.dirFileSize()
	s.l.Warnf("truncate: deleted node=%d, %d -> %d bytes", nodeBefore, before, after)
	return &MaintenanceResult{
		NodeDeleted: nodeBefore,
		BytesBefore: before,
		BytesAfter:  after,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

func (s *Store) ResetStats() {
	s.stats.Reset()
}
