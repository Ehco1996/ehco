package web

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	Hostname, _ = os.Hostname()

	CurTCPNum = prometheus.NewGauge(prometheus.GaugeOpts{
		Help: "当前tcp链接数",
		Name: "current_tcp_num",
		ConstLabels: map[string]string{
			"hostname": Hostname,
		},
	})

	CurUDPNum = prometheus.NewGauge(prometheus.GaugeOpts{
		Help: "当前udp链接数",
		Name: "current_udp_num",
		ConstLabels: map[string]string{
			"hostname": Hostname,
		},
	})

	NetWorkTransmitBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "network_transmit_bytes",
		Help: "传输流量总量bytes",
		ConstLabels: map[string]string{
			"hostname": Hostname,
		},
	})
)

func init() {

	prometheus.MustRegister(CurTCPNum)
	prometheus.MustRegister(CurUDPNum)
	prometheus.MustRegister(NetWorkTransmitBytes)

}
