package store

import (
	"context"
	"errors"
	"sort"

	"github.com/nakabonne/tstorage"
)

const (
	MetricCPUUsage    = "cpu_usage"
	MetricMemoryUsage = "memory_usage"
	MetricDiskUsage   = "disk_usage"
	MetricNetworkIn   = "network_in"
	MetricNetworkOut  = "network_out"
)

// Sample is the raw input metrics point to be stored.
type Sample struct {
	Timestamp                int64   `json:"timestamp"`
	CpuUsagePercent          float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent       float64 `json:"memory_usage_percent"`
	DiskUsagePercent         float64 `json:"disk_usage_percent"`
	NetworkReceiveBytesRate  float64 `json:"network_receive_bytes_rate"`
	NetworkTransmitBytesRate float64 `json:"network_transmit_bytes_rate"`
}

// NodeMetrics represents a single point returned to the dashboard UI.
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

func (s *Store) AddNodeMetric(ctx context.Context, sample *Sample) error {
	defer track(&s.stats.AddNode)()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storage == nil {
		return errors.New("metrics store not initialized")
	}

	ts := sample.Timestamp
	rows := []tstorage.Row{
		{Metric: MetricCPUUsage, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: sample.CpuUsagePercent}},
		{Metric: MetricMemoryUsage, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: sample.MemoryUsagePercent}},
		{Metric: MetricDiskUsage, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: sample.DiskUsagePercent}},
		{Metric: MetricNetworkIn, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: sample.NetworkReceiveBytesRate}},
		{Metric: MetricNetworkOut, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: sample.NetworkTransmitBytesRate}},
	}
	return s.storage.InsertRows(rows)
}

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

func (s *Store) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	defer track(&s.stats.QueryNode)()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.storage == nil {
		return nil, errors.New("metrics store not initialized")
	}

	start := req.StartTimestamp
	end := req.EndTimestamp + 1
	if end <= start {
		end = start + 1
	}

	step := req.Step
	if step <= 1 {
		step = 1
	}

	buckets := make(map[int64]*bucketAgg)
	getBucket := func(ts int64) *bucketAgg {
		bTS := (ts / step) * step
		b, ok := buckets[bTS]
		if !ok {
			b = &bucketAgg{ts: bTS}
			buckets[bTS] = b
		}
		return b
	}

	addMetric := func(name string, apply func(b *bucketAgg, val float64)) error {
		points, err := s.storage.Select(name, nil, start, end)
		if err != nil {
			if errors.Is(err, tstorage.ErrNoDataPoints) {
				return nil
			}
			return err
		}
		for _, p := range points {
			apply(getBucket(p.Timestamp), p.Value)
		}
		return nil
	}

	if err := addMetric(MetricCPUUsage, func(b *bucketAgg, v float64) { b.cpuSum += v; b.cpuCount++ }); err != nil {
		return nil, err
	}
	if err := addMetric(MetricMemoryUsage, func(b *bucketAgg, v float64) { b.memSum += v; b.memCount++ }); err != nil {
		return nil, err
	}
	if err := addMetric(MetricDiskUsage, func(b *bucketAgg, v float64) { b.diskSum += v; b.diskCount++ }); err != nil {
		return nil, err
	}
	if err := addMetric(MetricNetworkIn, func(b *bucketAgg, v float64) { b.netInSum += v; b.netInCount++ }); err != nil {
		return nil, err
	}
	if err := addMetric(MetricNetworkOut, func(b *bucketAgg, v float64) { b.netOutSum += v; b.netOutCount++ }); err != nil {
		return nil, err
	}

	data := make([]NodeMetrics, 0, len(buckets))
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
