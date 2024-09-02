package metric_reader

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/zap"
)

type Reader interface {
	ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error)
}

type readerImpl struct {
	metricsURL string
	httpClient *http.Client

	lastMetrics     *NodeMetrics
	lastRuleMetrics map[string]*RuleMetrics // key: label value: RuleMetrics
	l               *zap.SugaredLogger
}

func NewReader(metricsURL string) *readerImpl {
	c := &http.Client{Timeout: 30 * time.Second}
	return &readerImpl{
		httpClient: c,
		metricsURL: metricsURL,
		l:          zap.S().Named("metric_reader"),
	}
}

func (b *readerImpl) ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error) {
	metricMap, err := b.fetchMetrics(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to fetch metrics")
	}
	nm := &NodeMetrics{SyncTime: time.Now()}
	if err := b.ParseNodeMetrics(metricMap, nm); err != nil {
		return nil, nil, err
	}

	rm := make(map[string]*RuleMetrics)
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
	requiredMetrics := []string{
		"node_cpu_seconds_total",
		"node_load1", "node_load5", "node_load15",
		"node_memory_total_bytes", "node_memory_active_bytes", "node_memory_wired_bytes",
		"node_memory_MemTotal_bytes", "node_memory_MemAvailable_bytes",
		"node_filesystem_size_bytes", "node_filesystem_avail_bytes",
		"node_network_receive_bytes_total", "node_network_transmit_bytes_total",
	}

	for _, metricName := range requiredMetrics {
		metricFamily, ok := metricMap[metricName]
		if !ok {
			continue
		}

		for _, metric := range metricFamily.Metric {
			labels := make(map[string]string)
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			value := getMetricValue(metric, metricFamily.GetType())

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
			case "node_memory_active_bytes", "node_memory_wired_bytes":
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
		}
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

	return nil
}

func (b *readerImpl) ParseRuleMetrics(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	requiredMetrics := []string{
		"ehco_traffic_current_connection_count",
		"ehco_traffic_network_transmit_bytes",
		"ehco_ping_response_duration_milliseconds",
		"ehco_traffic_handshake_duration_milliseconds",
	}
	for _, metricName := range requiredMetrics {
		metricFamily, ok := metricMap[metricName]
		if !ok {
			continue
		}
		for _, metric := range metricFamily.Metric {
			labels := make(map[string]string)
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			value := getMetricValue(metric, metricFamily.GetType())
			label, ok := labels["label"]
			if !ok || label == "" {
				continue
			}

			if _, ok := rm[label]; !ok {
				rm[label] = &RuleMetrics{
					Label:                   label,
					PingMetrics:             make(map[string]*PingMetric),
					TCPConnectionCount:      make(map[string]float64),
					TCPHandShakeDuration:    make(map[string]float64),
					TCPNetworkTransmitBytes: make(map[string]float64),
					UDPConnectionCount:      make(map[string]float64),
					UDPHandShakeDuration:    make(map[string]float64),
					UDPNetworkTransmitBytes: make(map[string]float64),
				}
			}

			switch metricName {
			case "ehco_traffic_current_connection_count":
				key := labels["remote"]
				if labels["conn_type"] == "tcp" {
					rm[label].TCPConnectionCount[key] = value
				} else {
					rm[label].UDPConnectionCount[key] = value
				}
			case "ehco_traffic_network_transmit_bytes":
				key := labels["remote"]
				if labels["flow"] == "read" {
					if labels["conn_type"] == "tcp" {
						rm[label].TCPNetworkTransmitBytes[key] += value
					} else {
						rm[label].UDPNetworkTransmitBytes[key] += value
					}
				}
			case "ehco_ping_response_duration_milliseconds":
				target := labels["ip"]
				rm[label].PingMetrics[target] = &PingMetric{
					Latency: value,
					Target:  labels["ip"],
				}
			case "ehco_traffic_handshake_duration_milliseconds":
				key := labels["remote"]
				if labels["conn_type"] == "tcp" {
					rm[label].TCPHandShakeDuration[key] = value
				} else {
					rm[label].UDPHandShakeDuration[key] = value
				}
			}
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

func calculatePercentile(histogram *dto.Histogram, percentile float64) float64 {
	if histogram == nil {
		return 0
	}
	totalSamples := histogram.GetSampleCount()
	targetSample := percentile * float64(totalSamples)
	cumulativeCount := uint64(0)
	var lastBucketBound float64

	for _, bucket := range histogram.Bucket {
		cumulativeCount += bucket.GetCumulativeCount()
		if float64(cumulativeCount) >= targetSample {
			// Linear interpolation between bucket boundaries
			if bucket.GetCumulativeCount() > 0 && lastBucketBound != bucket.GetUpperBound() {
				return lastBucketBound + (float64(targetSample-float64(cumulativeCount-bucket.GetCumulativeCount()))/float64(bucket.GetCumulativeCount()))*(bucket.GetUpperBound()-lastBucketBound)
			} else {
				return bucket.GetUpperBound()
			}
		}
		lastBucketBound = bucket.GetUpperBound()
	}
	return math.NaN()
}

func getMetricValue(metric *dto.Metric, metricType dto.MetricType) float64 {
	switch metricType {
	case dto.MetricType_COUNTER:
		return metric.Counter.GetValue()
	case dto.MetricType_GAUGE:
		return metric.Gauge.GetValue()
	case dto.MetricType_HISTOGRAM:
		histogram := metric.Histogram
		if histogram != nil {
			return calculatePercentile(histogram, 0.9)
		}
	}
	return 0
}
