package metric_reader

import (
	"time"
)

type NodeMetrics struct {
	// cpu
	CpuCoreCount    int     `json:"cpu_core_count"`
	CpuLoadInfo     string  `json:"cpu_load_info"`
	CpuUsagePercent float64 `json:"cpu_usage_percent"`

	// memory
	MemoryTotalBytes   float64 `json:"memory_total_bytes"`
	MemoryUsageBytes   float64 `json:"memory_usage_bytes"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`

	// disk
	DiskTotalBytes   float64 `json:"disk_total_bytes"`
	DiskUsageBytes   float64 `json:"disk_usage_bytes"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`

	// network
	NetworkReceiveBytesTotal  float64 `json:"network_receive_bytes_total"`
	NetworkTransmitBytesTotal float64 `json:"network_transmit_bytes_total"`
	NetworkReceiveBytesRate   float64 `json:"network_receive_bytes_rate"`
	NetworkTransmitBytesRate  float64 `json:"network_transmit_bytes_rate"`

	// ping
	PingMetrics []PingMetric `json:"ping_metrics"`

	SyncTime time.Time
}

type PingMetric struct {
	Latency float64 `json:"latency"` // in ms
	Target  string  `json:"target"`
}
