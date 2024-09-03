package ms

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"github.com/Ehco1996/ehco/pkg/metric_reader"
)

type NodeMetrics struct {
	Timestamp int64 `json:"timestamp"`

	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`  // bytes per second
	NetworkOut  float64 `json:"network_out"` // bytes per second
}

type QueryNodeMetricsReq struct {
	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryNodeMetricsResp struct {
	TOTAL int           `json:"total"`
	Data  []NodeMetrics `json:"data"`
}

type RuleMetricsData struct {
	Timestamp               int64  `json:"timestamp"`
	Label                   string `json:"label"`
	Remote                  string `json:"remote"`
	PingLatency             int64  `json:"ping_latency"`
	TCPConnectionCount      int64  `json:"tcp_connection_count"`
	TCPHandshakeDuration    int64  `json:"tcp_handshake_duration"`
	TCPNetworkTransmitBytes int64  `json:"tcp_network_transmit_bytes"`
	UDPConnectionCount      int64  `json:"udp_connection_count"`
	UDPHandshakeDuration    int64  `json:"udp_handshake_duration"`
	UDPNetworkTransmitBytes int64  `json:"udp_network_transmit_bytes"`
}

type QueryRuleMetricsReq struct {
	RuleLabel string

	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryRuleMetricsResp struct {
	TOTAL int               `json:"total"`
	Data  []RuleMetricsData `json:"data"`
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
            tcp_handshake_duration INTEGER,
            tcp_network_transmit_bytes INTEGER,
            udp_connection_count INTEGER,
            udp_handshake_duration INTEGER,
            udp_network_transmit_bytes INTEGER,
            PRIMARY KEY (timestamp, label, remote)
        )
    `); err != nil {
		return err
	}
	return nil
}

func (ms *MetricsStore) AddNodeMetric(m *metric_reader.NodeMetrics) error {
	_, err := ms.db.Exec(`
    INSERT OR REPLACE INTO node_metrics (timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out)
    VALUES (?, ?, ?, ?, ?, ?)
`, m.SyncTime.Unix(), m.CpuUsagePercent, m.MemoryUsagePercent, m.DiskUsagePercent, m.NetworkReceiveBytesRate, m.NetworkTransmitBytesRate)
	return err
}

func (ms *MetricsStore) AddRuleMetric(ctx context.Context, rm *metric_reader.RuleMetrics) error {
	tx, err := ms.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
        INSERT OR REPLACE INTO rule_metrics
        (timestamp, label, remote, ping_latency,
         tcp_connection_count, tcp_handshake_duration, tcp_network_transmit_bytes,
         udp_connection_count, udp_handshake_duration, udp_network_transmit_bytes)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck

	for remote, pingMetric := range rm.PingMetrics {
		_, err := stmt.ExecContext(ctx, rm.SyncTime.Unix(), rm.Label, remote, pingMetric.Latency,
			rm.TCPConnectionCount[remote], rm.TCPHandShakeDuration[remote], rm.TCPNetworkTransmitBytes[remote],
			rm.UDPConnectionCount[remote], rm.UDPHandShakeDuration[remote], rm.UDPNetworkTransmitBytes[remote])
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	rows, err := ms.db.QueryContext(ctx, `
	SELECT timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out
	FROM node_metrics
	WHERE timestamp >= ? AND timestamp <= ?
	ORDER BY timestamp DESC
	LIMIT ?
`, req.StartTimestamp, req.EndTimestamp, req.Num)
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

func (ms *MetricsStore) QueryRuleMetric(ctx context.Context, req *QueryRuleMetricsReq) (*QueryRuleMetricsResp, error) {
	rows, err := ms.db.Query(`
        SELECT timestamp, label, remote, ping_latency,
               tcp_connection_count, tcp_handshake_duration, tcp_network_transmit_bytes,
               udp_connection_count, udp_handshake_duration, udp_network_transmit_bytes
        FROM rule_metrics
        WHERE timestamp >= ? AND timestamp <= ? AND label = ?
        ORDER BY timestamp DESC
        LIMIT ?
    `, req.StartTimestamp, req.EndTimestamp, req.RuleLabel, req.Num)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var resp QueryRuleMetricsResp
	for rows.Next() {
		var m RuleMetricsData
		if err := rows.Scan(&m.Timestamp, &m.Label, &m.Remote, &m.PingLatency,
			&m.TCPConnectionCount, &m.TCPHandshakeDuration, &m.TCPNetworkTransmitBytes,
			&m.UDPConnectionCount, &m.UDPHandshakeDuration, &m.UDPNetworkTransmitBytes); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, m)
	}
	resp.TOTAL = len(resp.Data)
	return &resp, nil
}
