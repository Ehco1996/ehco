package store

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nakabonne/tstorage"
	"go.uber.org/zap"
)

// defaultRetentionDays mirrors the 30d window for metric retention.
const defaultRetentionDays = 30

type Store struct {
	mu      sync.RWMutex
	storage tstorage.Storage
	dirPath string

	l *zap.SugaredLogger

	// stats is the latency/throughput recorder shared by every public
	// method on this store. See stats.go.
	stats Stats

	closeOnce sync.Once
}

func NewStore(dirPath string) (*Store, error) {
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

	s := &Store{
		dirPath: dirPath,
		storage: storage,
		l:       zap.S().Named("store"),
	}

	return s, nil
}

func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.storage != nil {
			err = s.storage.Close()
			s.storage = nil
		}
	})
	return err
}

func (s *Store) countSamples() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storage == nil {
		return 0
	}
	cutoff := time.Now().Add(-defaultRetentionDays * 24 * time.Hour).Unix()
	pts, err := s.storage.Select(MetricCPUUsage, nil, cutoff, time.Now().Unix()+3600)
	if err != nil {
		return 0
	}
	return int64(len(pts))
}

func (s *Store) countPartitionsAndFiles() (partitions int, files int, totalBytes int64) {
	entries, err := os.ReadDir(s.dirPath)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			partitions++
			subEntries, _ := os.ReadDir(filepath.Join(s.dirPath, e.Name()))
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

func (s *Store) dirFileSize() int64 {
	_, _, totalBytes := s.countPartitionsAndFiles()
	return totalBytes
}
