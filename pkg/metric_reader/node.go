package metric_reader

import (
	"fmt"
	"math"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
)

const (
	metricCPUSecondsTotal           = "node_cpu_seconds_total"
	metricLoad1                     = "node_load1"
	metricLoad5                     = "node_load5"
	metricLoad15                    = "node_load15"
	metricMemoryTotalBytes          = "node_memory_total_bytes"
	metricMemoryActiveBytes         = "node_memory_active_bytes"
	metricMemoryWiredBytes          = "node_memory_wired_bytes"
	metricMemoryMemTotalBytes       = "node_memory_MemTotal_bytes"
	metricMemoryMemAvailableBytes   = "node_memory_MemAvailable_bytes"
	metricFilesystemSizeBytes       = "node_filesystem_size_bytes"
	metricFilesystemAvailBytes      = "node_filesystem_avail_bytes"
	metricNetworkReceiveBytesTotal  = "node_network_receive_bytes_total"
	metricNetworkTransmitBytesTotal = "node_network_transmit_bytes_total"
)

type NodeMetrics struct {
	// cpu
	CpuCoreCount    int     `json:"cpu_core_count"`
	CpuLoadInfo     string  `json:"cpu_load_info"`
	CpuUsagePercent float64 `json:"cpu_usage_percent"`

	// memory
	MemoryTotalBytes   int64   `json:"memory_total_bytes"`
	MemoryUsageBytes   int64   `json:"memory_usage_bytes"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`

	// disk
	DiskTotalBytes   int64   `json:"disk_total_bytes"`
	DiskUsageBytes   int64   `json:"disk_usage_bytes"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`

	// network
	NetworkReceiveBytesTotal  int64   `json:"network_receive_bytes_total"`
	NetworkTransmitBytesTotal int64   `json:"network_transmit_bytes_total"`
	NetworkReceiveBytesRate   float64 `json:"network_receive_bytes_rate"`
	NetworkTransmitBytesRate  float64 `json:"network_transmit_bytes_rate"`

	SyncTime time.Time
}
type cpuStats struct {
	totalTime float64
	idleTime  float64
	cores     int
}

func (b *readerImpl) ParseNodeMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	isMac := metricMap[metricMemoryTotalBytes] != nil
	cpu := &cpuStats{}

	b.processCPUMetrics(metricMap, cpu)
	b.processMemoryMetrics(metricMap, nm, isMac)
	b.processDiskMetrics(metricMap, nm)
	b.processNetworkMetrics(metricMap, nm)
	b.processLoadMetrics(metricMap, nm)

	b.calculateFinalMetrics(nm, cpu)
	return nil
}

func (b *readerImpl) processCPUMetrics(metricMap map[string]*dto.MetricFamily, cpu *cpuStats) {
	if cpuMetric, ok := metricMap[metricCPUSecondsTotal]; ok {
		for _, metric := range cpuMetric.Metric {
			value := getMetricValue(metric, cpuMetric.GetType())
			cpu.totalTime += value
			if getLabel(metric, "mode") == "idle" {
				cpu.idleTime += value
				cpu.cores++
			}
		}
	}
}

func (b *readerImpl) processMemoryMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics, isMac bool) {
	if isMac {
		nm.MemoryTotalBytes = sumInt64Metric(metricMap, metricMemoryTotalBytes)
		nm.MemoryUsageBytes = sumInt64Metric(metricMap, metricMemoryActiveBytes) + sumInt64Metric(metricMap, metricMemoryWiredBytes)
	} else {
		nm.MemoryTotalBytes = sumInt64Metric(metricMap, metricMemoryMemTotalBytes)
		availableMemory := sumInt64Metric(metricMap, metricMemoryMemAvailableBytes)
		nm.MemoryUsageBytes = nm.MemoryTotalBytes - availableMemory
	}
}

func (b *readerImpl) processDiskMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) {
	if metric, ok := metricMap[metricFilesystemSizeBytes]; ok {
		for _, m := range metric.Metric {
			if getLabel(m, "mountpoint") == "/" {
				nm.DiskTotalBytes = int64(getMetricValue(m, metric.GetType()))
				break
			}
		}
	}

	if metric, ok := metricMap[metricFilesystemAvailBytes]; ok {
		for _, m := range metric.Metric {
			if getLabel(m, "mountpoint") == "/" {
				availableDisk := int64(getMetricValue(m, metric.GetType()))
				nm.DiskUsageBytes = nm.DiskTotalBytes - availableDisk
				break
			}
		}
	}
}

func (b *readerImpl) processNetworkMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) {
	nm.NetworkReceiveBytesTotal = sumInt64Metric(metricMap, metricNetworkReceiveBytesTotal)
	nm.NetworkTransmitBytesTotal = sumInt64Metric(metricMap, metricNetworkTransmitBytesTotal)
}

func (b *readerImpl) processLoadMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) {
	loads := []string{metricLoad1, metricLoad5, metricLoad15}
	for _, load := range loads {
		value := sumFloat64Metric(metricMap, load)
		nm.CpuLoadInfo += fmt.Sprintf("%.2f|", value)
	}
	nm.CpuLoadInfo = strings.TrimRight(nm.CpuLoadInfo, "|")
}

func (b *readerImpl) calculateFinalMetrics(nm *NodeMetrics, cpu *cpuStats) {
	nm.CpuCoreCount = cpu.cores
	nm.CpuUsagePercent = 100 * (cpu.totalTime - cpu.idleTime) / cpu.totalTime
	nm.MemoryUsagePercent = 100 * float64(nm.MemoryUsageBytes) / float64(nm.MemoryTotalBytes)
	nm.DiskUsagePercent = 100 * float64(nm.DiskUsageBytes) / float64(nm.DiskTotalBytes)

	nm.CpuUsagePercent = math.Round(nm.CpuUsagePercent*100) / 100
	nm.MemoryUsagePercent = math.Round(nm.MemoryUsagePercent*100) / 100
	nm.DiskUsagePercent = math.Round(nm.DiskUsagePercent*100) / 100

	if b.lastMetrics != nil {
		duration := time.Since(b.lastMetrics.SyncTime).Seconds()
		if duration > 0.1 {
			nm.NetworkReceiveBytesRate = math.Max(0, float64(nm.NetworkReceiveBytesTotal-b.lastMetrics.NetworkReceiveBytesTotal)/duration)
			nm.NetworkTransmitBytesRate = math.Max(0, float64(nm.NetworkTransmitBytesTotal-b.lastMetrics.NetworkTransmitBytesTotal)/duration)
			nm.NetworkReceiveBytesRate = math.Round(nm.NetworkReceiveBytesRate)
			nm.NetworkTransmitBytesRate = math.Round(nm.NetworkTransmitBytesRate)
		}
	}
}

func sumInt64Metric(metricMap map[string]*dto.MetricFamily, metricName string) int64 {
	ret := int64(0)
	if metric, ok := metricMap[metricName]; ok && len(metric.Metric) > 0 {
		for _, m := range metric.Metric {
			ret += int64(getMetricValue(m, metric.GetType()))
		}
	}
	return ret
}

func sumFloat64Metric(metricMap map[string]*dto.MetricFamily, metricName string) float64 {
	ret := float64(0)
	if metric, ok := metricMap[metricName]; ok && len(metric.Metric) > 0 {
		for _, m := range metric.Metric {
			ret += getMetricValue(m, metric.GetType())
		}
	}
	return ret
}

func getLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.Label {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
