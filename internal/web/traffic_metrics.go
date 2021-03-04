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

	CurTCPNum = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "traffic",
		Help:        "当前tcp链接数",
		Name:        "current_tcp_num",
		ConstLabels: ConstLabels,
	})

	CurUDPNum = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "traffic",
		Help:        "当前udp链接数",
		Name:        "current_udp_num",
		ConstLabels: ConstLabels,
	})

	NetWorkTransmitBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   "traffic",
		Name:        "network_transmit_bytes",
		Help:        "传输流量总量bytes",
		ConstLabels: ConstLabels,
	})
)
