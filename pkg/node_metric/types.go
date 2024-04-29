package node_metric

import (
	"fmt"

	"github.com/Ehco1996/ehco/pkg/bytes"
)

type NodeMetrics struct {
	// cpu
	CpuCoreCount    int     `json:"cpu_core_count"`
	CpuUsagePercent float64 `json:"cpu_usage_percent"`
	CpuLoadInfo     string  `json:"cpu_load_info"`

	// memory
	MemoryTotalBytes float64 `json:"memory_total_bytes"`
	MemoryUsageBytes float64 `json:"memory_usage_bytes"`

	// disk
	DiskTotalBytes float64 `json:"disk_total_bytes"`
	DiskUsageBytes float64 `json:"disk_usage_bytes"`

	// network
	NetworkReceiveBytesTotal  float64 `json:"network_receive_bytes_total"`
	NetworkTransmitBytesTotal float64 `json:"network_transmit_bytes_total"`
}

func (n *NodeMetrics) ToString() string {
	// cpu
	cpu := fmt.Sprintf("\ncpu core count: %d\ncpu usage percent: %.2f \ncpu load info: %s\n",
		n.CpuCoreCount, n.CpuUsagePercent, n.CpuLoadInfo)
	// memory
	memory := fmt.Sprintf("memory total bytes: %s\nmemory usage bytes: %s\n",
		bytes.PrettyByteSize(n.MemoryTotalBytes),
		bytes.PrettyByteSize(n.MemoryUsageBytes))
	// disk
	disk := fmt.Sprintf("disk total bytes: %s\ndisk usage bytes: %s\n",
		bytes.PrettyByteSize(n.DiskTotalBytes),
		bytes.PrettyByteSize(n.DiskUsageBytes))
	// network
	network := fmt.Sprintf("network receive bytes total: %s\nnetwork transmit bytes total: %s\n",
		bytes.PrettyByteSize(n.NetworkReceiveBytesTotal),
		bytes.PrettyByteSize(n.NetworkTransmitBytesTotal))
	return cpu + memory + disk + network
}
