package node_metric

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/pkg/bytes"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
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
	UploadBandwidthBytes   float64 `json:"upload_bandwidth_bytes"`
	DownloadBandwidthBytes float64 `json:"download_bandwidth_bytes"`
}

func (n *NodeMetrics) TOString() string {

	// cpu
	cpu := fmt.Sprintf("cpu core count: %d\ncpu usage percent: %.2f \ncpu load info: %s\n", n.CpuCoreCount, n.CpuUsagePercent, n.CpuLoadInfo)

	// memory
	memory := fmt.Sprintf("memory total bytes: %s\nmemory usage bytes: %s\n",
		bytes.PrettyByteSize(n.MemoryTotalBytes),
		bytes.PrettyByteSize(n.MemoryUsageBytes))

	// disk
	disk := fmt.Sprintf("disk total bytes: %s\ndisk usage bytes: %s\n",
		bytes.PrettyByteSize(n.DiskTotalBytes),
		bytes.PrettyByteSize(n.DiskUsageBytes))

	return cpu + memory + disk
}

type nodeMetricReader struct {
	httpClient *http.Client
	metricsURL string
}

func NewNodeMetricReader(metricsURL string) *nodeMetricReader {
	c := &http.Client{Timeout: 30 * time.Second}
	return &nodeMetricReader{
		httpClient: c,
		metricsURL: metricsURL,
	}
}

func (b *nodeMetricReader) parseCpuInfo(lines *[]string, nm *NodeMetrics) error {
	var (
		cpuCores  int
		cpuLoad1  float64
		cpuLoad5  float64
		cpuLoad15 float64

		totalIdleTime float64
		totalCpuTime  float64
	)

	for _, line := range *lines {
		if strings.HasPrefix(line, "node_cpu_seconds_total") {
			time := parseFloat(strings.Split(line, " ")[1])
			totalCpuTime += time
			if strings.Contains(line, `mode="idle"`) {
				totalIdleTime += time
				cpuCores++
			}
		}
		if strings.HasPrefix(line, "node_load1") && !strings.HasPrefix(line, "node_load15") {
			load1 := strings.Split(line, " ")[1]
			cpuLoad1 = parseFloat(load1)
		}
		if strings.HasPrefix(line, "node_load5") {
			load5 := strings.Split(line, " ")[1]
			cpuLoad5 = parseFloat(load5)
		}
		if strings.HasPrefix(line, "node_load15") {
			load15 := strings.Split(line, " ")[1]
			cpuLoad15 = parseFloat(load15)
		}
	}
	nm.CpuCoreCount = cpuCores
	nm.CpuUsagePercent = 100 * (totalCpuTime - totalIdleTime) / totalCpuTime
	nm.CpuLoadInfo = fmt.Sprintf("%.2f | %.2f | %.2f", cpuLoad1, cpuLoad5, cpuLoad15)
	return nil
}

func (b *nodeMetricReader) parseMemoryInfo(lines *[]string, nm *NodeMetrics) error {
	var (
		totalMem, usageMem, freeMem float64
	)
	for _, line := range *lines {
		// handle macos
		if strings.HasPrefix(line, "node_memory_total_bytes") {
			totalMem = parseFloat(strings.Split(line, " ")[1])
		}
		if strings.HasPrefix(line, "node_memory_active_bytes") {
			usageMem += parseFloat(strings.Split(line, " ")[1])
		}
		if strings.HasPrefix(line, "node_memory_inactive_bytes") {
			usageMem += parseFloat(strings.Split(line, " ")[1])
		}

		// handle linux
		if strings.HasPrefix(line, "node_memory_MemTotal_bytes") {
			totalMem = parseFloat(strings.Split(line, " ")[1])
		}
		if strings.HasPrefix(line, "node_memory_MemAvailable_bytes") {
			freeMem = parseFloat(strings.Split(line, " ")[1])
		}
	}
	nm.MemoryTotalBytes = totalMem
	if usageMem == 0 {
		usageMem = totalMem - freeMem
	}
	nm.MemoryUsageBytes = usageMem
	return nil
}

func getDiskName(devicePath string) string {
	parts := strings.Split(devicePath, "/")
	lastPart := parts[len(parts)-1]
	diskName := strings.TrimRightFunc(lastPart, func(r rune) bool {
		return r != 's' && (r >= '0' && r <= '9')
	})
	return diskName
}

func (b *nodeMetricReader) parseDiskInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	// handle disk total
	diskMap := make(map[string]float64)
	totalMetric, ok := metricMap["node_filesystem_size_bytes"]
	if !ok {
		return fmt.Errorf("node_filesystem_size_bytes not found")
	}

	forMac := false
	for _, metric := range totalMetric.Metric {
		g := metric.GetGauge()
		disk := ""
		for _, label := range metric.GetLabel() {
			if label.GetName() == "device" {
				disk = getDiskName(label.GetValue())
			}
			if label.GetName() == "fstype" && label.GetValue() == "apfs" {
				forMac = true
			}
		}
		diskMap[getDiskName(disk)] = g.GetValue()
	}
	// 对于macos，的apfs文件系统，可能会有多个相同大小的磁盘，这是因为apfs磁盘（卷）会共享物理磁盘
	seenVal := map[float64]bool{}
	for _, val := range diskMap {
		if seenVal[val] && forMac {
			continue
		}
		nm.DiskTotalBytes += val
		seenVal[val] = true
	}

	// handle disk usage
	var availDisk float64
	usageMetric, ok := metricMap["node_filesystem_avail_bytes"]
	if !ok {
		return fmt.Errorf("node_filesystem_avail_bytes not found")
	}
	diskMap = make(map[string]float64)
	for _, metric := range usageMetric.Metric {
		g := metric.GetGauge()
		disk := ""
		for _, label := range metric.GetLabel() {
			if *label.Name == "device" {
				disk = getDiskName(label.GetValue())
			}
		}
		diskMap[disk] = g.GetValue()
	}
	seenVal = map[float64]bool{}
	for _, val := range diskMap {
		if seenVal[val] && forMac {
			continue
		}
		availDisk += val
		seenVal[val] = true
	}
	nm.DiskUsageBytes = nm.DiskTotalBytes - availDisk
	return nil
}

func (b *nodeMetricReader) RecordOnce(ctx context.Context) (*NodeMetrics, error) {
	response, err := b.httpClient.Get(b.metricsURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(body), "\n")

	nm := &NodeMetrics{}
	if err := b.parseCpuInfo(&lines, nm); err != nil {
		return nil, err
	}
	if err := b.parseMemoryInfo(&lines, nm); err != nil {
		return nil, err
	}
	var parser expfmt.TextParser
	parsed, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	if err := b.parseDiskInfo(parsed, nm); err != nil {
		return nil, err
	}

	return nm, nil
}

func parseFloat(s string) float64 {
	value, _ := strconv.ParseFloat(s, 64)
	return value
}
