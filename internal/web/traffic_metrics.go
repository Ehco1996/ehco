package web

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	Hostname, _ = os.Hostname()

	ConstLabels = map[string]string{
		"hostname": Hostname,
	}
	EhcoAlive = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   "",
		Name:        "alive_state",
		Help:        "ehco 存活状态",
		ConstLabels: ConstLabels})

	CurConnectionCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "current_connection_count",
		Help:        "当前链接数",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE, METRIC_LABEL_CONN_TYPE})

	NetWorkTransmitBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "network_transmit_bytes",
		Help:        "传输流量总量bytes",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE, METRIC_LABEL_CONN_TYPE, METRIC_LABEL_CONN_FLOW})

	HandShakeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Namespace:   METRIC_NS,
		Name:        "handshake_duration",
		Help:        "握手时间ms",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE})
)
