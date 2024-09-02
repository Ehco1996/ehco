package metric_reader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
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
	metricMap, err := b.fetchMetrics(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to fetch metrics")
	}
	nm := &NodeMetrics{SyncTime: time.Now()}
	rm := make(map[string]*RuleMetrics)

	if err := b.ParseNodeMetrics(metricMap, nm); err != nil {
		return nil, nil, err
	}
	if err := b.ParseRuleMetrics(metricMap, rm); err != nil {
		return nil, nil, err
	}

	b.lastMetrics = nm
	b.lastRuleMetrics = rm
	return nm, rm, nil
}

func (b *readerImpl) ParseNodeMetrics(metricMap map[string]*dto.MetricFamily, nm *NodeMetrics) error {
	var totalIdleTime, totalCpuTime float64
	cpuCores := 0

	isMac := false
	if _, ok := metricMap["node_memory_total_bytes"]; ok {
		isMac = true
	}
	err := b.parseMetrics(metricMap, func(metricName string, value float64, labels map[string]string) {
		switch metricName {
		case "node_cpu_seconds_total":
			totalCpuTime += value
			if labels["mode"] == "idle" {
				totalIdleTime += value
				cpuCores++
			}
		case "node_load1", "node_load5", "node_load15":
			nm.CpuLoadInfo += fmt.Sprintf("%.2f|", value)
		case "node_memory_total_bytes":
			if isMac {
				nm.MemoryTotalBytes = value
			}
		case "node_memory_active_bytes":
			if isMac {
				nm.MemoryUsageBytes += value
			}
		case "node_memory_wired_bytes":
			if isMac {
				nm.MemoryUsageBytes += value
			}
		case "node_memory_MemTotal_bytes":
			if !isMac {
				nm.MemoryTotalBytes = value
			}
		case "node_memory_MemAvailable_bytes":
			if !isMac {
				nm.MemoryUsageBytes = nm.MemoryTotalBytes - value
			}
		case "node_filesystem_size_bytes":
			nm.DiskTotalBytes += value
		case "node_filesystem_avail_bytes":
			nm.DiskUsageBytes += (nm.DiskTotalBytes - value)
		case "node_network_receive_bytes_total":
			nm.NetworkReceiveBytesTotal += value
		case "node_network_transmit_bytes_total":
			nm.NetworkTransmitBytesTotal += value
		}
		// calculate usage
		nm.CpuCoreCount = cpuCores
		nm.CpuUsagePercent = 100 * (totalCpuTime - totalIdleTime) / totalCpuTime
		nm.CpuLoadInfo = strings.TrimRight(nm.CpuLoadInfo, "|")
		nm.MemoryUsagePercent = 100 * nm.MemoryUsageBytes / nm.MemoryTotalBytes
		nm.DiskUsagePercent = 100 * nm.DiskUsageBytes / nm.DiskTotalBytes
		if b.lastMetrics != nil {
			nm.NetworkReceiveBytesRate = (nm.NetworkReceiveBytesTotal - b.lastMetrics.NetworkReceiveBytesTotal) / time.Since(b.lastMetrics.SyncTime).Seconds()
			nm.NetworkTransmitBytesRate = (nm.NetworkTransmitBytesTotal - b.lastMetrics.NetworkTransmitBytesTotal) / time.Since(b.lastMetrics.SyncTime).Seconds()
		}
	})

	println("after parse",
		"cpu", nm.CpuCoreCount, nm.CpuLoadInfo, nm.CpuUsagePercent, "\n",
		"memory", nm.MemoryUsagePercent, "\n",
		"disk", nm.DiskUsagePercent, "\n",
		nm.NetworkReceiveBytesTotal, nm.NetworkTransmitBytesTotal,
		nm.NetworkReceiveBytesRate, nm.NetworkTransmitBytesRate)

	return err
}

func (b *readerImpl) ParseRuleMetrics(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	return b.parseMetrics(metricMap, func(metricName string, value float64, labels map[string]string) {
		label := labels["label"]
		if _, ok := rm[label]; !ok {
			rm[label] = &RuleMetrics{
				Label:                label,
				CurConnectionCount:   make(map[string]float64),
				HandShakeDuration:    make(map[string]*dto.Histogram),
				NetWorkTransmitBytes: make(map[string]float64),
			}
		}

		switch metricName {
		case "ehco_traffic_current_connection_count":
			key := fmt.Sprintf("%s:%s", labels["conn_type"], labels["remote"])
			rm[label].CurConnectionCount[key] = value
		case "ehco_traffic_network_transmit_bytes":
			key := fmt.Sprintf("%s:%s:%s", labels["conn_type"], labels["flow"], labels["remote"])
			rm[label].NetWorkTransmitBytes[key] = value
		case "ehco_ping_response_duration_seconds":
			rm[label].PingMetrics = append(rm[label].PingMetrics, PingMetric{
				Latency: value * 1000, // 转换为毫秒
				Target:  labels["ip"],
			})
		}
	})
}

func (b *readerImpl) parseMetrics(metricMap map[string]*dto.MetricFamily, handler func(string, float64, map[string]string)) error {
	for metricName, metricFamily := range metricMap {
		for _, metric := range metricFamily.Metric {
			labels := make(map[string]string)
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}

			var value float64
			switch metricFamily.GetType() {
			case dto.MetricType_COUNTER:
				value = metric.Counter.GetValue()
			case dto.MetricType_GAUGE:
				value = metric.Gauge.GetValue()
			case dto.MetricType_HISTOGRAM:
				// TODO 对于 Histogram，我们可能需要特殊处理
				handler(metricName, 0, labels)
				continue
			default:
				continue // 跳过不支持的类型
			}
			handler(metricName, value, labels)
		}
	}
	return nil
}

func (r *readerImpl) fetchMetrics(ctx context.Context) (map[string]*dto.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.metricsURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(strings.NewReader(string(body)))
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
