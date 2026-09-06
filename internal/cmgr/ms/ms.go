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

func (ms *MetricsStore) dirFileSize() int64 {
	var totalSize int64
	_ = filepath.Walk(ms.dirPath, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	return totalSize
}
