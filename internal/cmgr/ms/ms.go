package ms

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// defaultRetentionDays is how far back cleanOldData and the
// CleanupOlderThan default keep rows. Mirrors the historical 30d window.
const defaultRetentionDays = 30

type MetricsStore struct {
	db     *sql.DB
	dbPath string

	l *zap.SugaredLogger

	// stats is the latency/throughput recorder shared by every public
	// method on this store. See stats.go.
	stats Stats

	// nodeRows / ruleRows are best-effort row-count caches kept in
	// sync with INSERT / DELETE so Health() doesn't need a per-call
	// SELECT COUNT(*). Refreshed on startup, recomputed after
	// Cleanup / Truncate / Vacuum where the exact post-state matters.
	// INSERT OR REPLACE on a duplicate PK can briefly overcount; the
	// drift is bounded and resets every time Recount() runs.
	nodeRows atomic.Int64
	ruleRows atomic.Int64
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
	if err := ms.recountRows(); err != nil {
		return nil, err
	}
	return ms, nil
}

func (ms *MetricsStore) Close() error {
	return ms.db.Close()
}

func (ms *MetricsStore) cleanOldData() error {
	defer track(&ms.stats.Cleanup)()
	cutoff := time.Now().AddDate(0, 0, -defaultRetentionDays).Unix()
	_, _, err := ms.deleteOlderThan(cutoff)
	return err
}

// deleteOlderThan runs the two-table prune and returns the number of
// rows removed from each. Centralises the SQL so cleanOldData and the
// CleanupOlderThan API path stay consistent.
func (ms *MetricsStore) deleteOlderThan(cutoff int64) (nodeDeleted, ruleDeleted int64, err error) {
	res, err := ms.db.Exec("DELETE FROM node_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, 0, err
	}
	nodeDeleted, _ = res.RowsAffected()

	res, err = ms.db.Exec("DELETE FROM rule_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return nodeDeleted, 0, err
	}
	ruleDeleted, _ = res.RowsAffected()

	ms.nodeRows.Add(-nodeDeleted)
	ms.ruleRows.Add(-ruleDeleted)
	ms.l.Infof("pruned node_metrics=%d rule_metrics=%d (cutoff=%d)", nodeDeleted, ruleDeleted, cutoff)
	return nodeDeleted, ruleDeleted, nil
}

// recountRows refreshes the cached row counts from the source of truth.
// Cheap on startup (db usually small, even at 30d full retention); we
// also call it after Truncate / Vacuum where the cache may have drifted
// or been wiped wholesale.
func (ms *MetricsStore) recountRows() error {
	var nodeRows, ruleRows int64
	if err := ms.db.QueryRow("SELECT COUNT(*) FROM node_metrics").Scan(&nodeRows); err != nil {
		return err
	}
	if err := ms.db.QueryRow("SELECT COUNT(*) FROM rule_metrics").Scan(&ruleRows); err != nil {
		return err
	}
	ms.nodeRows.Store(nodeRows)
	ms.ruleRows.Store(ruleRows)
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
