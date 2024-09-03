package metric_reader

import (
	"time"

	dto "github.com/prometheus/client_model/go"
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

type PingMetric struct {
	Latency int64  `json:"latency"` // in ms
	Target  string `json:"target"`
}

type RuleMetrics struct {
	Label string // rule label

	PingMetrics map[string]*PingMetric // key: remote

	TCPConnectionCount      map[string]int64 // key: remote
	TCPHandShakeDuration    map[string]int64 // key: remote in ms
	TCPNetworkTransmitBytes map[string]int64 // key: remote

	UDPConnectionCount      map[string]int64 // key: remote
	UDPHandShakeDuration    map[string]int64 // key: remote in ms
	UDPNetworkTransmitBytes map[string]int64 // key: remote

	SyncTime time.Time
}

func (b *readerImpl) ParseRuleMetrics(metricMap map[string]*dto.MetricFamily, rm map[string]*RuleMetrics) error {
	requiredMetrics := []string{
		metricConnectionCount,
		metricNetworkTransmit,
		metricPingResponse,
		metricHandshakeDuration,
	}

	for _, metricName := range requiredMetrics {
		metricFamily, ok := metricMap[metricName]
		if !ok {
			continue
		}

		for _, metric := range metricFamily.Metric {
			labels := getLabelMap(metric)
			value := int64(getMetricValue(metric, metricFamily.GetType()))
			label, ok := labels[labelKey]
			if !ok || label == "" {
				continue
			}

			ruleMetric := b.ensureRuleMetric(rm, label)

			switch metricName {
			case metricConnectionCount:
				b.updateConnectionCount(ruleMetric, labels, value)
			case metricNetworkTransmit:
				b.updateNetworkTransmit(ruleMetric, labels, value)
			case metricPingResponse:
				b.updatePingMetrics(ruleMetric, labels, value)
			case metricHandshakeDuration:
				b.updateHandshakeDuration(ruleMetric, labels, value)
			}
		}
	}
	return nil
}

func (b *readerImpl) ensureRuleMetric(rm map[string]*RuleMetrics, label string) *RuleMetrics {
	if _, ok := rm[label]; !ok {
		rm[label] = &RuleMetrics{
			Label:                   label,
			PingMetrics:             make(map[string]*PingMetric),
			TCPConnectionCount:      make(map[string]int64),
			TCPHandShakeDuration:    make(map[string]int64),
			TCPNetworkTransmitBytes: make(map[string]int64),
			UDPConnectionCount:      make(map[string]int64),
			UDPHandShakeDuration:    make(map[string]int64),
			UDPNetworkTransmitBytes: make(map[string]int64),

			SyncTime: time.Now(),
		}
	}
	return rm[label]
}

func (b *readerImpl) updateConnectionCount(rm *RuleMetrics, labels map[string]string, value int64) {
	key := labels[remoteKey]
	switch labels[connTypeKey] {
	case "tcp":
		rm.TCPConnectionCount[key] = value
	default:
		rm.UDPConnectionCount[key] = value
	}
}

func (b *readerImpl) updateNetworkTransmit(rm *RuleMetrics, labels map[string]string, value int64) {
	if labels[flowKey] == "read" {
		key := labels[remoteKey]
		switch labels[connTypeKey] {
		case "tcp":
			rm.TCPNetworkTransmitBytes[key] += value
		default:
			rm.UDPNetworkTransmitBytes[key] += value
		}
	}
}

func (b *readerImpl) updatePingMetrics(rm *RuleMetrics, labels map[string]string, value int64) {
	remote := labels[remoteKey]
	rm.PingMetrics[remote] = &PingMetric{
		Latency: value,
		Target:  labels[ipKey],
	}
}

func (b *readerImpl) updateHandshakeDuration(rm *RuleMetrics, labels map[string]string, value int64) {
	key := labels[remoteKey]
	switch labels[connTypeKey] {
	case "tcp":
		rm.TCPHandShakeDuration[key] = value
	default:
		rm.UDPHandShakeDuration[key] = value
	}
}

func getLabelMap(metric *dto.Metric) map[string]string {
	labels := make(map[string]string)
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}
