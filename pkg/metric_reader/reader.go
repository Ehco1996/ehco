package metric_reader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type Reader interface {
	ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error)
}

type readerImpl struct {
	metricsURL string
	httpClient *http.Client

	lastMetrics     *NodeMetrics
	lastRuleMetrics map[string]*RuleMetrics
}

func NewReader(metricsURL string) *readerImpl {
	c := &http.Client{Timeout: 30 * time.Second}
	return &readerImpl{
		httpClient: c,
		metricsURL: metricsURL,
	}
}

func (b *readerImpl) ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error) {
	response, err := b.httpClient.Get(b.metricsURL)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	var parser expfmt.TextParser
	parsed, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		return nil, nil, err
	}

	nm := &NodeMetrics{SyncTime: time.Now()}
	rm := make(map[string]*RuleMetrics)

	if err := b.ParseNodeMetrics(parsed, nm); err != nil {
		return nil, nil, err
	}
	if err := b.ParseRuleMetrics(parsed, rm); err != nil {
		return nil, nil, err
	}

	b.lastMetrics = nm
	b.lastRuleMetrics = rm
	return nm, rm, nil
}

func (b *readerImpl) ParseNodeMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	if err := b.parseCpuInfo(metricMap, nm); err != nil {
		return err
	}
	if err := b.parseMemoryInfo(metricMap, nm); err != nil {
		return err
	}
	if err := b.parseDiskInfo(metricMap, nm); err != nil {
		return err
	}
	if err := b.parseNetworkInfo(metricMap, nm); err != nil {
		return err
	}
	return nil
}

func (b *readerImpl) ParseRuleMetrics(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	// Parse CurConnectionCount
	if err := b.parseCurConnectionCount(metricMap, rm); err != nil {
		return err
	}

	// Parse HandShakeDuration
	if err := b.parseHandShakeDuration(metricMap, rm); err != nil {
		return err
	}

	// Parse NetWorkTransmitBytes
	if err := b.parseNetWorkTransmitBytes(metricMap, rm); err != nil {
		return err
	}
	// Parse PingMetrics
	if err := b.parsePingInfo(metricMap, rm); err != nil {
		return err
	}
	return nil
}

func (b *readerImpl) parseCpuInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
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

func (b *readerImpl) parseMemoryInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
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

		err = handleMetric("node_memory_wired_bytes", func(val float64) {
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
	if nm.MemoryTotalBytes != 0 {
		nm.MemoryUsagePercent = 100 * nm.MemoryUsageBytes / nm.MemoryTotalBytes
	}
	return nil
}

func (b *readerImpl) parseDiskInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
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
		// 对于 macos 的 apfs 文件系统，可能会有多个相同大小的磁盘，这是因为 apfs 磁盘（卷）会共享物理磁盘
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

	var availBytes float64
	err = handleMetric("node_filesystem_avail_bytes", func(val float64) {
		availBytes += val
	})
	if err != nil {
		return err
	}
	nm.DiskUsageBytes = nm.DiskTotalBytes - availBytes
	if nm.DiskTotalBytes != 0 {
		nm.DiskUsagePercent = 100 * nm.DiskUsageBytes / nm.DiskTotalBytes
	}
	return nil
}

func (b *readerImpl) parseNetworkInfo(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	now := time.Now()
	handleMetric := func(metricName string, handleValue func(float64)) error {
		metric, ok := metricMap[metricName]
		if !ok {
			return fmt.Errorf("%s not found", metricName)
		}
		for _, m := range metric.Metric {
			g := m.GetCounter()
			handleValue(g.GetValue())
		}
		return nil
	}

	err := handleMetric("node_network_receive_bytes_total", func(val float64) {
		nm.NetworkReceiveBytesTotal += val
	})
	if err != nil {
		return err
	}

	err = handleMetric("node_network_transmit_bytes_total", func(val float64) {
		nm.NetworkTransmitBytesTotal += val
	})
	if err != nil {
		return err
	}

	if b.lastMetrics != nil {
		passedTime := now.Sub(b.lastMetrics.SyncTime).Seconds()
		nm.NetworkReceiveBytesRate = (nm.NetworkReceiveBytesTotal - b.lastMetrics.NetworkReceiveBytesTotal) / passedTime
		nm.NetworkTransmitBytesRate = (nm.NetworkTransmitBytesTotal - b.lastMetrics.NetworkTransmitBytesTotal) / passedTime
	}
	return nil
}

func (b *readerImpl) parseCurConnectionCount(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	metric, ok := metricMap["ehco_traffic_current_connection_count"]
	if !ok {
		return nil
	}

	for _, m := range metric.Metric {
		label := ""
		connType := ""
		remote := ""
		for _, l := range m.Label {
			switch l.GetName() {
			case "label":
				label = l.GetValue()
			case "conn_type":
				connType = l.GetValue()
			case "remote":
				remote = l.GetValue()
			}
		}

		if _, ok := rm[label]; !ok {
			rm[label] = &RuleMetrics{
				Label:                label,
				CurConnectionCount:   make(map[string]float64),
				HandShakeDuration:    make(map[string]*dto.Histogram),
				NetWorkTransmitBytes: make(map[string]float64),
			}
		}

		key := fmt.Sprintf("%s:%s", connType, remote)
		rm[label].CurConnectionCount[key] = m.Gauge.GetValue()
	}

	return nil
}

func (b *readerImpl) parseHandShakeDuration(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	metric, ok := metricMap["ehco_traffic_handshake_duration"]
	if !ok {
		return nil
	}

	for _, m := range metric.Metric {
		label := ""
		connType := ""
		remote := ""
		for _, l := range m.Label {
			switch l.GetName() {
			case "label":
				label = l.GetValue()
			case "conn_type":
				connType = l.GetValue()
			case "remote":
				remote = l.GetValue()
			}
		}

		if _, ok := rm[label]; !ok {
			rm[label] = &RuleMetrics{
				Label:                label,
				CurConnectionCount:   make(map[string]float64),
				HandShakeDuration:    make(map[string]*dto.Histogram),
				NetWorkTransmitBytes: make(map[string]float64),
			}
		}

		key := fmt.Sprintf("%s:%s", connType, remote)
		rm[label].HandShakeDuration[key] = m.Histogram
	}

	return nil
}

func (b *readerImpl) parseNetWorkTransmitBytes(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	metric, ok := metricMap["ehco_traffic_network_transmit_bytes"]
	if !ok {
		return nil
	}

	for _, m := range metric.Metric {
		label := ""
		connType := ""
		flow := ""
		remote := ""
		for _, l := range m.Label {
			switch l.GetName() {
			case "label":
				label = l.GetValue()
			case "conn_type":
				connType = l.GetValue()
			case "flow":
				flow = l.GetValue()
			case "remote":
				remote = l.GetValue()
			}
		}

		if _, ok := rm[label]; !ok {
			rm[label] = &RuleMetrics{
				Label:                label,
				CurConnectionCount:   make(map[string]float64),
				HandShakeDuration:    make(map[string]*dto.Histogram),
				NetWorkTransmitBytes: make(map[string]float64),
			}
		}

		key := fmt.Sprintf("%s:%s:%s", connType, flow, remote)
		rm[label].NetWorkTransmitBytes[key] = m.Counter.GetValue()
	}

	return nil
}

func (b *readerImpl) parsePingInfo(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	metric, ok := metricMap["ehco_ping_response_duration_seconds"]
	if !ok {
		return nil
	}
	for _, m := range metric.Metric {
		g := m.GetHistogram()
		ruleLabel := ""
		ip := ""
		val := float64(g.GetSampleSum()) / float64(g.GetSampleCount()) * 1000 // to ms
		for _, label := range m.GetLabel() {
			if label.GetName() == "ip" {
				ip = label.GetValue()
			} else if label.GetName() == "label" {
				ruleLabel = label.GetValue()
			}
		}

		if _, ok := rm[ruleLabel]; !ok {
			rm[ruleLabel] = &RuleMetrics{
				Label:                ruleLabel,
				CurConnectionCount:   make(map[string]float64),
				HandShakeDuration:    make(map[string]*dto.Histogram),
				NetWorkTransmitBytes: make(map[string]float64),
			}
		}
		rm[ruleLabel].PingMetrics = append(rm[ruleLabel].PingMetrics, PingMetric{Latency: val, Target: ip})
	}
	return nil
}
