package ms

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

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
	if err := ms.cleanOldData(); err != nil {
		return nil, err
	}
	return ms, nil
}

func (ms *MetricsStore) cleanOldData() error {
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Unix()

	// 清理 node_metrics 表
	_, err := ms.db.Exec("DELETE FROM node_metrics WHERE timestamp < ?", thirtyDaysAgo)
	if err != nil {
		return err
	}

	// 清理 rule_metrics 表
	_, err = ms.db.Exec("DELETE FROM rule_metrics WHERE timestamp < ?", thirtyDaysAgo)
	if err != nil {
		return err
	}

	ms.l.Infof("Cleaned data older than 30 days")
	return nil
}

func (ms *MetricsStore) initDB() error {
	// init NodeMetrics table
	if _, err := ms.db.Exec(`
        CREATE TABLE IF NOT EXISTS node_metrics (
            timestamp INTEGER,
            cpu_usage REAL,
            memory_usage REAL,
            disk_usage REAL,
            network_in REAL,
            network_out REAL,
            PRIMARY KEY (timestamp)
        )
    `); err != nil {
		return err
	}

	// init rule_metrics
	if _, err := ms.db.Exec(`
        CREATE TABLE IF NOT EXISTS rule_metrics (
            timestamp INTEGER,
            label TEXT,
            remote TEXT,
            ping_latency INTEGER,
            tcp_connection_count INTEGER,
            tcp_handshake_duration BIGINT,
            tcp_network_transmit_bytes BIGINT,
            udp_connection_count INTEGER,
            udp_handshake_duration BIGINT,
            udp_network_transmit_bytes BIGINT,
            PRIMARY KEY (timestamp, label, remote)
        )
    `); err != nil {
		return err
	}
	return nil
}
