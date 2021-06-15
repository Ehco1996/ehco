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
	EhcoAlive = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   METRIC_NS,
		Subsystem:   "",
		Name:        "alive_state",
		Help:        "ehco 存活状态",
		ConstLabels: ConstLabels})

	CurTCPNum = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "current_tcp_num",
		Help:        "当前tcp链接数",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE})

	CurUDPNum = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Help:        "当前udp链接数",
		Name:        "current_udp_num",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE})

	NetWorkTransmitBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "network_transmit_bytes",
		Help:        "传输流量总量bytes",
		ConstLabels: ConstLabels,
	}, []string{METRIC_LABEL_REMOTE})
)
