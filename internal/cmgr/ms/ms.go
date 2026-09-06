package ms

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nakabonne/tstorage"
	"go.uber.org/zap"
)

// defaultRetentionDays mirrors the historical 30d window.
const defaultRetentionDays = 30

type MetricsStore struct {
	mu      sync.RWMutex
	storage tstorage.Storage
	dirPath string

	l *zap.SugaredLogger

	// stats is the latency/throughput recorder shared by every public
	// method on this store. See stats.go.
	stats Stats

	// nodeRows is an in-memory sample-count tracker kept in sync with AddNodeMetric.
	nodeRows atomic.Int64

	closeOnce sync.Once
}

func NewMetricsStore(dirPath string) (*MetricsStore, error) {
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return nil, err
	}

	storage, err := tstorage.NewStorage(
		tstorage.WithDataPath(dirPath),
		tstorage.WithPartitionDuration(1*time.Hour),
		tstorage.WithRetention(defaultRetentionDays*24*time.Hour),
		tstorage.WithTimestampPrecision(tstorage.Seconds),
	)
	if err != nil {
		return nil, err
	}

	ms := &MetricsStore{
		dirPath: dirPath,
		storage: storage,
		l:       zap.S().Named("ms"),
	}

	// Initialize nodeRows from existing samples in storage if available
	if pts, err := storage.Select("cpu_usage", nil, 0, time.Now().Unix()+3600); err == nil {
		ms.nodeRows.Store(int64(len(pts)))
	}

	return ms, nil
}

func (ms *MetricsStore) Close() error {
	var err error
	ms.closeOnce.Do(func() {
		ms.mu.Lock()
		defer ms.mu.Unlock()
		if ms.storage != nil {
			err = ms.storage.Close()
			ms.storage = nil
		}
	})
	return err
}

func (ms *MetricsStore) countPartitionsAndFiles() (partitions int, files int, totalBytes int64) {
	entries, err := os.ReadDir(ms.dirPath)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			partitions++
			subEntries, _ := os.ReadDir(filepath.Join(ms.dirPath, e.Name()))
			for _, se := range subEntries {
				if !se.IsDir() {
					files++
					if fi, err := se.Info(); err == nil {
						totalBytes += fi.Size()
					}
				}
			}
		} else {
			files++
			if fi, err := e.Info(); err == nil {
				totalBytes += fi.Size()
			}
		}
	}
	return
}

func (ms *MetricsStore) dirFileSize() int64 {
	_, _, totalBytes := ms.countPartitionsAndFiles()
	return totalBytes
}
