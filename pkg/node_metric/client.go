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

func (b *nodeMetricReader) parseCpuInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	handleMetric := func(metricName string, handleValue func(float64, string)) error {
		metric, ok := metricMap[metricName]
		if !ok {
			return fmt.Errorf("%s not found", metricName)
		}

		for _, m := range metric.Metric {
			g := m.GetCounter()
			mode := ""
			for _, label := range m.GetLabel() {
				if label.GetName() == "mode" {
					mode = label.GetValue()
				}
			}
			handleValue(g.GetValue(), mode)
		}
		return nil
	}

	var (
		totalIdleTime float64
		totalCpuTime  float64
		cpuCores      int
	)

	err := handleMetric("node_cpu_seconds_total", func(val float64, mode string) {
		totalCpuTime += val
		if mode == "idle" {
			totalIdleTime += val
			cpuCores++
		}
	})
	if err != nil {
		return err
	}

	nm.CpuCoreCount = cpuCores
	nm.CpuUsagePercent = 100 * (totalCpuTime - totalIdleTime) / totalCpuTime
	for _, load := range []string{"1", "5", "15"} {
		loadMetricName := fmt.Sprintf("node_load%s", load)
		loadMetric, ok := metricMap[loadMetricName]
		if !ok {
			return fmt.Errorf("%s not found", loadMetricName)
		}
		for _, m := range loadMetric.Metric {
			g := m.GetGauge()
			nm.CpuLoadInfo += fmt.Sprintf("%.2f|", g.GetValue())
		}
	}
	nm.CpuLoadInfo = strings.TrimRight(nm.CpuLoadInfo, "|")
	return nil
}

func (b *nodeMetricReader) parseMemoryInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	handleMetric := func(metricName string, handleValue func(float64)) error {
		metric, ok := metricMap[metricName]
		if !ok {
			return fmt.Errorf("%s not found", metricName)
		}
		for _, m := range metric.Metric {
			g := m.GetGauge()
			handleValue(g.GetValue())
		}
		return nil
	}

	isMac := false
	if _, ok := metricMap["node_memory_total_bytes"]; ok {
		isMac = true
	}

	if isMac {
		err := handleMetric("node_memory_total_bytes", func(val float64) {
			nm.MemoryTotalBytes = val
		})
		if err != nil {
			return err
		}

		err = handleMetric("node_memory_active_bytes", func(val float64) {
			nm.MemoryUsageBytes += val
		})
		if err != nil {
			return err
		}

		err = handleMetric("node_memory_inactive_bytes", func(val float64) {
			nm.MemoryUsageBytes += val
		})
		if err != nil {
			return err
		}
	} else {
		err := handleMetric("node_memory_MemTotal_bytes", func(val float64) {
			nm.MemoryTotalBytes = val
		})
		if err != nil {
			return err
		}

		err = handleMetric("node_memory_MemAvailable_bytes", func(val float64) {
			nm.MemoryUsageBytes = nm.MemoryTotalBytes - val
		})
		if err != nil {
			return err
		}
	}

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
	handleMetric := func(metricName string, handleValue func(float64)) error {
		forMac := false
		diskMap := make(map[string]float64)
		metric, ok := metricMap[metricName]
		if !ok {
			return fmt.Errorf("%s not found", metricName)
		}
		for _, m := range metric.Metric {
			g := m.GetGauge()
			disk := ""
			for _, label := range m.GetLabel() {
				if label.GetName() == "device" {
					disk = getDiskName(label.GetValue())
				}
				if label.GetName() == "fstype" && label.GetValue() == "apfs" {
					forMac = true
				}
			}
			diskMap[disk] = g.GetValue()
		}
		// 对于macos，的apfs文件系统，可能会有多个相同大小的磁盘，这是因为apfs磁盘（卷）会共享物理磁盘
		seenVal := map[float64]bool{}
		for _, val := range diskMap {
			if seenVal[val] && forMac {
				continue
			}
			handleValue(val)
			seenVal[val] = true
		}
		return nil
	}

	err := handleMetric("node_filesystem_size_bytes", func(val float64) {
		nm.DiskTotalBytes += val
	})
	if err != nil {
		return err
	}

	err = handleMetric("node_filesystem_avail_bytes", func(val float64) {
		nm.DiskUsageBytes += val
	})
	if err != nil {
		return err
	}
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
	var parser expfmt.TextParser
	parsed, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	nm := &NodeMetrics{}
	if err := b.parseCpuInfo(parsed, nm); err != nil {
		return nil, err
	}
	if err := b.parseMemoryInfo(parsed, nm); err != nil {
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
