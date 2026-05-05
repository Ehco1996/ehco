package sampler

import "time"

type NodeMetrics struct {
	CpuCoreCount    int     `json:"cpu_core_count"`
	CpuLoadInfo     string  `json:"cpu_load_info"`
	CpuUsagePercent float64 `json:"cpu_usage_percent"`

	MemoryTotalBytes   int64   `json:"memory_total_bytes"`
	MemoryUsageBytes   int64   `json:"memory_usage_bytes"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`

	DiskTotalBytes   int64   `json:"disk_total_bytes"`
	DiskUsageBytes   int64   `json:"disk_usage_bytes"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`

	NetworkReceiveBytesTotal  int64   `json:"network_receive_bytes_total"`
	NetworkTransmitBytesTotal int64   `json:"network_transmit_bytes_total"`
	NetworkReceiveBytesRate   float64 `json:"network_receive_bytes_rate"`
	NetworkTransmitBytesRate  float64 `json:"network_transmit_bytes_rate"`

	SyncTime time.Time
}
