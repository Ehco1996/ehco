package ms

import (
	"context"
	"database/sql"

	"github.com/Ehco1996/ehco/internal/cmgr/sampler"
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

func (ms *MetricsStore) AddNodeMetric(ctx context.Context, m *sampler.NodeMetrics) error {
	defer track(&ms.stats.AddNode)()
	_, err := ms.db.ExecContext(ctx, `
    INSERT OR REPLACE INTO node_metrics (timestamp, cpu_usage, memory_usage, disk_usage, network_in, network_out)
    VALUES (?, ?, ?, ?, ?, ?)
`, m.SyncTime.Unix(), m.CpuUsagePercent, m.MemoryUsagePercent, m.DiskUsagePercent, m.NetworkReceiveBytesRate, m.NetworkTransmitBytesRate)
	if err != nil {
		return err
	}
	// INSERT OR REPLACE may collapse duplicates rather than add a row;
	// the count is best-effort and is reconciled by recountRows on
	// next Vacuum / Truncate / restart.
	ms.nodeRows.Add(1)
	return nil
}

func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	defer track(&ms.stats.QueryNode)()
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
