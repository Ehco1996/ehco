package ms

import (
	"context"
	"database/sql"

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
	// Step buckets samples into N-second windows when > 1, averaging
	// every gauge field per bucket. Lets the SPA pull 7d/30d windows
	// without dragging back hundreds of thousands of raw points.
	Step int64
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
	Remote    string

	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
	// Step keeps the last sample per (label, remote) within each
	// N-second bucket. Counter-style fields (transmit bytes) keep
	// monotonic semantics so the SPA's delta-on-consecutive-points
	// trend math still works after bucketing.
	Step int64
}

type QueryRuleMetricsResp struct {
	TOTAL int               `json:"total"`
	Data  []RuleMetricsData `json:"data"`
}

func (ms *MetricsStore) AddNodeMetric(ctx context.Context, m *metric_reader.NodeMetrics) error {
	_, err := ms.db.ExecContext(ctx, `
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
	var (
		rows *sql.Rows
		err  error
	)
	if req.Step > 1 {
		// Floor each timestamp to a step-second bucket and average every
		// gauge field. Cheaper than rolling a separate downsample table
		// for the windows we care about (≤30d).
		rows, err = ms.db.QueryContext(ctx, `
		SELECT (timestamp/?)*? AS bucket_ts,
		       AVG(cpu_usage), AVG(memory_usage), AVG(disk_usage),
		       AVG(network_in), AVG(network_out)
		FROM node_metrics
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY bucket_ts
		ORDER BY bucket_ts DESC
		LIMIT ?
	`, req.Step, req.Step, req.StartTimestamp, req.EndTimestamp, req.Num)
	} else {
		rows, err = ms.db.QueryContext(ctx, `
		SELECT timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out
		FROM node_metrics
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, req.StartTimestamp, req.EndTimestamp, req.Num)
	}
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
	// Bucketed mode: keep the last sample per (label, remote) inside each
	// step-second window. The bytes columns are monotonic counters, so
	// last-of-bucket preserves the deltas the SPA computes — averaging
	// them would smear the curve.
	var (
		query string
		args  []interface{}
	)
	if req.Step > 1 {
		query = `
        SELECT timestamp, label, remote, ping_latency,
               tcp_connection_count, tcp_handshake_duration, tcp_network_transmit_bytes,
               udp_connection_count, udp_handshake_duration, udp_network_transmit_bytes
        FROM rule_metrics
        WHERE rowid IN (
            SELECT MAX(rowid) FROM rule_metrics
            WHERE timestamp >= ? AND timestamp <= ?`
		args = []interface{}{req.StartTimestamp, req.EndTimestamp}
		if req.RuleLabel != "" {
			query += " AND label = ?"
			args = append(args, req.RuleLabel)
		}
		if req.Remote != "" {
			query += " AND remote = ?"
			args = append(args, req.Remote)
		}
		query += `
            GROUP BY (timestamp/?), label, remote
        )
        ORDER BY timestamp DESC
        LIMIT ?
    `
		args = append(args, req.Step, req.Num)
	} else {
		query = `
        SELECT timestamp, label, remote, ping_latency,
               tcp_connection_count, tcp_handshake_duration, tcp_network_transmit_bytes,
               udp_connection_count, udp_handshake_duration, udp_network_transmit_bytes
        FROM rule_metrics
        WHERE timestamp >= ? AND timestamp <= ?
    `
		args = []interface{}{req.StartTimestamp, req.EndTimestamp}
		if req.RuleLabel != "" {
			query += " AND label = ?"
			args = append(args, req.RuleLabel)
		}
		if req.Remote != "" {
			query += " AND remote = ?"
			args = append(args, req.Remote)
		}
		query += `
        ORDER BY timestamp DESC
        LIMIT ?
    `
		args = append(args, req.Num)
	}

	rows, err := ms.db.Query(query, args...)
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
