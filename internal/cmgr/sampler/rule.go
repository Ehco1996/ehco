package sampler

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
)

const (
	metricConnectionCount   = "ehco_traffic_current_connection_count"
	metricNetworkTransmit   = "ehco_traffic_network_transmit_bytes"
	metricPingResponse      = "ehco_ping_response_duration_milliseconds"
	metricHandshakeDuration = "ehco_traffic_handshake_duration_milliseconds"

	labelKey    = "label"
	remoteKey   = "remote"
	connTypeKey = "conn_type"
	flowKey     = "flow"
	ipKey       = "ip"
)

// RuleSampler reads ehco's own traffic / ping counters directly from the
// Prometheus default gatherer instead of taking a self-HTTP-scrape +
// expfmt round trip. The numbers come from the same client_golang
// collectors backing the public /metrics endpoint, so values match what
// an external scraper would see.
type RuleSampler struct {
	g prometheus.Gatherer
	l *zap.SugaredLogger
}

func NewRuleSampler() *RuleSampler {
	return &RuleSampler{
		g: prometheus.DefaultGatherer,
		l: zap.S().Named("sampler.rule"),
	}
}

func (s *RuleSampler) Sample() (map[string]*RuleMetrics, error) {
	families, err := s.g.Gather()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		byName[f.GetName()] = f
	}

	rm := make(map[string]*RuleMetrics)
	now := time.Now()

	for _, name := range []string{metricConnectionCount, metricNetworkTransmit, metricPingResponse, metricHandshakeDuration} {
		fam, ok := byName[name]
		if !ok {
			continue
		}
		for _, m := range fam.Metric {
			labels := labelMap(m)
			label := labels[labelKey]
			if label == "" {
				continue
			}
			value := int64(metricValue(m, fam.GetType()))
			r := ensure(rm, label, now)
			switch name {
			case metricConnectionCount:
				if labels[connTypeKey] == "tcp" {
					r.TCPConnectionCount[labels[remoteKey]] = value
				} else {
					r.UDPConnectionCount[labels[remoteKey]] = value
				}
			case metricNetworkTransmit:
				if labels[flowKey] != "read" {
					continue
				}
				if labels[connTypeKey] == "tcp" {
					r.TCPNetworkTransmitBytes[labels[remoteKey]] += value
				} else {
					r.UDPNetworkTransmitBytes[labels[remoteKey]] += value
				}
			case metricPingResponse:
				r.PingMetrics[labels[remoteKey]] = &PingMetric{
					Latency: value,
					Target:  labels[ipKey],
				}
			case metricHandshakeDuration:
				if labels[connTypeKey] == "tcp" {
					r.TCPHandShakeDuration[labels[remoteKey]] = value
				} else {
					r.UDPHandShakeDuration[labels[remoteKey]] = value
				}
			}
		}
	}
	return rm, nil
}

func ensure(rm map[string]*RuleMetrics, label string, now time.Time) *RuleMetrics {
	if r, ok := rm[label]; ok {
		return r
	}
	r := &RuleMetrics{
		Label:                   label,
		PingMetrics:             make(map[string]*PingMetric),
		TCPConnectionCount:      make(map[string]int64),
		TCPHandShakeDuration:    make(map[string]int64),
		TCPNetworkTransmitBytes: make(map[string]int64),
		UDPConnectionCount:      make(map[string]int64),
		UDPHandShakeDuration:    make(map[string]int64),
		UDPNetworkTransmitBytes: make(map[string]int64),
		SyncTime:                now,
	}
	rm[label] = r
	return r
}

func labelMap(m *dto.Metric) map[string]string {
	out := make(map[string]string, len(m.Label))
	for _, l := range m.Label {
		out[l.GetName()] = l.GetValue()
	}
	return out
}

// metricValue extracts a scalar from a *dto.Metric. Histograms collapse
// to a p90 quantile (interpolated across buckets) — the previous
// expfmt-based reader did the same so persisted ping / handshake series
// stay continuous across the migration.
func metricValue(m *dto.Metric, t dto.MetricType) float64 {
	switch t {
	case dto.MetricType_COUNTER:
		if m.Counter != nil {
			return m.Counter.GetValue()
		}
	case dto.MetricType_GAUGE:
		if m.Gauge != nil {
			return m.Gauge.GetValue()
		}
	case dto.MetricType_HISTOGRAM:
		if m.Histogram != nil {
			return histogramPercentile(m.Histogram, 0.9)
		}
	case dto.MetricType_UNTYPED:
		if m.Untyped != nil {
			return m.Untyped.GetValue()
		}
	}
	return 0
}

func histogramPercentile(h *dto.Histogram, percentile float64) float64 {
	if h == nil {
		return 0
	}
	total := h.GetSampleCount()
	if total == 0 {
		return 0
	}
	target := percentile * float64(total)
	var (
		cumulative      uint64
		lastBucketBound float64
	)
	for _, b := range h.Bucket {
		cumulative += b.GetCumulativeCount()
		if float64(cumulative) >= target {
			if b.GetCumulativeCount() > 0 && lastBucketBound != b.GetUpperBound() {
				return lastBucketBound + (target-float64(cumulative-b.GetCumulativeCount()))/
					float64(b.GetCumulativeCount())*(b.GetUpperBound()-lastBucketBound)
			}
			return b.GetUpperBound()
		}
		lastBucketBound = b.GetUpperBound()
	}
	return 0
}
