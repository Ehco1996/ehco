package ms

import (
	"context"
	"errors"
	"sort"

	"github.com/Ehco1996/ehco/internal/cmgr/sampler"
	"github.com/nakabonne/tstorage"
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
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.storage == nil {
		return errors.New("metrics store not initialized")
	}

	ts := m.SyncTime.Unix()
	rows := []tstorage.Row{
		{Metric: "cpu_usage", DataPoint: tstorage.DataPoint{Timestamp: ts, Value: m.CpuUsagePercent}},
		{Metric: "memory_usage", DataPoint: tstorage.DataPoint{Timestamp: ts, Value: m.MemoryUsagePercent}},
		{Metric: "disk_usage", DataPoint: tstorage.DataPoint{Timestamp: ts, Value: m.DiskUsagePercent}},
		{Metric: "network_in", DataPoint: tstorage.DataPoint{Timestamp: ts, Value: m.NetworkReceiveBytesRate}},
		{Metric: "network_out", DataPoint: tstorage.DataPoint{Timestamp: ts, Value: m.NetworkTransmitBytesRate}},
	}
	if err := ms.storage.InsertRows(rows); err != nil {
		return err
	}
	ms.nodeRows.Add(1)
	return nil
}

func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	defer track(&ms.stats.QueryNode)()
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.storage == nil {
		return nil, errors.New("metrics store not initialized")
	}

	start := req.StartTimestamp
	end := req.EndTimestamp + 1 // tstorage.Select end is exclusive
	if end <= start {
		end = start + 1
	}

	selectMetric := func(metric string) ([]*tstorage.DataPoint, error) {
		points, err := ms.storage.Select(metric, nil, start, end)
		if err != nil {
			if errors.Is(err, tstorage.ErrNoDataPoints) {
				return nil, nil
			}
			return nil, err
		}
		return points, nil
	}

	cpuPoints, err := selectMetric("cpu_usage")
	if err != nil {
		return nil, err
	}
	memPoints, err := selectMetric("memory_usage")
	if err != nil {
		return nil, err
	}
	diskPoints, err := selectMetric("disk_usage")
	if err != nil {
		return nil, err
	}
	netInPoints, err := selectMetric("network_in")
	if err != nil {
		return nil, err
	}
	netOutPoints, err := selectMetric("network_out")
	if err != nil {
		return nil, err
	}

	var data []NodeMetrics
	if req.Step > 1 {
		type bucketAgg struct {
			ts          int64
			cpuSum      float64
			cpuCount    int
			memSum      float64
			memCount    int
			diskSum     float64
			diskCount   int
			netInSum    float64
			netInCount  int
			netOutSum   float64
			netOutCount int
		}
		buckets := make(map[int64]*bucketAgg)
		getBucket := func(ts int64) *bucketAgg {
			bTS := (ts / req.Step) * req.Step
			b, ok := buckets[bTS]
			if !ok {
				b = &bucketAgg{ts: bTS}
				buckets[bTS] = b
			}
			return b
		}
		for _, p := range cpuPoints {
			b := getBucket(p.Timestamp)
			b.cpuSum += p.Value
			b.cpuCount++
		}
		for _, p := range memPoints {
			b := getBucket(p.Timestamp)
			b.memSum += p.Value
			b.memCount++
		}
		for _, p := range diskPoints {
			b := getBucket(p.Timestamp)
			b.diskSum += p.Value
			b.diskCount++
		}
		for _, p := range netInPoints {
			b := getBucket(p.Timestamp)
			b.netInSum += p.Value
			b.netInCount++
		}
		for _, p := range netOutPoints {
			b := getBucket(p.Timestamp)
			b.netOutSum += p.Value
			b.netOutCount++
		}

		data = make([]NodeMetrics, 0, len(buckets))
		for _, b := range buckets {
			m := NodeMetrics{Timestamp: b.ts}
			if b.cpuCount > 0 {
				m.CPUUsage = b.cpuSum / float64(b.cpuCount)
			}
			if b.memCount > 0 {
				m.MemoryUsage = b.memSum / float64(b.memCount)
			}
			if b.diskCount > 0 {
				m.DiskUsage = b.diskSum / float64(b.diskCount)
			}
			if b.netInCount > 0 {
				m.NetworkIn = b.netInSum / float64(b.netInCount)
			}
			if b.netOutCount > 0 {
				m.NetworkOut = b.netOutSum / float64(b.netOutCount)
			}
			data = append(data, m)
		}
	} else {
		pointMap := make(map[int64]*NodeMetrics)
		getPoint := func(ts int64) *NodeMetrics {
			m, ok := pointMap[ts]
			if !ok {
				m = &NodeMetrics{Timestamp: ts}
				pointMap[ts] = m
			}
			return m
		}
		for _, p := range cpuPoints {
			getPoint(p.Timestamp).CPUUsage = p.Value
		}
		for _, p := range memPoints {
			getPoint(p.Timestamp).MemoryUsage = p.Value
		}
		for _, p := range diskPoints {
			getPoint(p.Timestamp).DiskUsage = p.Value
		}
		for _, p := range netInPoints {
			getPoint(p.Timestamp).NetworkIn = p.Value
		}
		for _, p := range netOutPoints {
			getPoint(p.Timestamp).NetworkOut = p.Value
		}

		data = make([]NodeMetrics, 0, len(pointMap))
		for _, m := range pointMap {
			data = append(data, *m)
		}
	}

	// Sort DESC by Timestamp (matching SQL ORDER BY bucket_ts/timestamp DESC)
	sort.Slice(data, func(i, j int) bool {
		return data[i].Timestamp > data[j].Timestamp
	})

	if req.Num > 0 && int64(len(data)) > req.Num {
		data = data[:req.Num]
	}

	return &QueryNodeMetricsResp{
		TOTAL: len(data),
		Data:  data,
	}, nil
}
