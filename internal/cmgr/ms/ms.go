package ms

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"github.com/Ehco1996/ehco/pkg/metric_reader"
)

type NodeMetrics struct {
	Timestamp int64 `json:"timestamp"`

	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`
	NetworkOut  float64 `json:"network_out"`
}

type QueryNodeMetricsReq struct {
	StartTimestamp int64 `json:"start_ts"`
	EndTimestamp   int64 `json:"end_ts"`

	Latest bool `json:"latest"` // whether to refresh the cache and get the latest data
}
type QueryNodeMetricsResp struct {
	TOTAL int           `json:"total"`
	Data  []NodeMetrics `json:"data"`
}

type MetricsStore struct {
	db     *sql.DB
	dbPath string

	l *zap.SugaredLogger
}

func NewMetricsStore(dbPath string) (*MetricsStore, error) {
	// ensure the directory exists
	dirPath := filepath.Dir(dbPath)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return nil, err
	}
	// create db file if not exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		f, err := os.Create(dbPath)
		if err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	ms := &MetricsStore{dbPath: dbPath, db: db, l: zap.S().Named("ms")}
	if err := ms.initDB(); err != nil {
		return nil, err
	}
	return ms, nil
}

func (ms *MetricsStore) initDB() error {
	// init NodeMetrics table
	_, err := ms.db.Exec(`
        CREATE TABLE IF NOT EXISTS node_metrics (
            timestamp INTEGER,
            cpu_usage REAL,
            memory_usage REAL,
            disk_usage REAL,
            network_in REAL,
            network_out REAL,
            PRIMARY KEY (timestamp)
        )
    `)
	return err
}

func (ms *MetricsStore) AddNodeMetric(m *metric_reader.NodeMetrics) error {
	_, err := ms.db.Exec(`
    INSERT OR REPLACE INTO node_metrics (timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out)
    VALUES (?, ?, ?, ?, ?, ?)
`, m.SyncTime.Unix(), m.CpuUsagePercent, m.MemoryUsagePercent, m.DiskUsagePercent, m.NetworkReceiveBytesRate, m.NetworkTransmitBytesRate)
	return err
}

func (ms *MetricsStore) QueryNodeMetric(startTime, endTime time.Time, num int) (*QueryNodeMetricsResp, error) {
	rows, err := ms.db.Query(`
	SELECT timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out
	FROM node_metrics
	WHERE timestamp >= ? AND timestamp <= ?
	ORDER BY timestamp DESC
	LIMIT ?
`, startTime.Unix(), endTime.Unix(), num)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var resp QueryNodeMetricsResp
	for rows.Next() {
		var m NodeMetrics
		if err := rows.Scan(&m.Timestamp, &m.CPUUsage, &m.MemoryUsage, &m.DiskUsage, &m.NetworkIn, &m.NetworkOut); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, m)
	}
	resp.TOTAL = len(resp.Data)
	return &resp, nil
}
