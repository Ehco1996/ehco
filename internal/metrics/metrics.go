package metrics

import (
	"os"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	METRIC_NS                = "ehco"
	METRIC_SUBSYSTEM_TRAFFIC = "traffic"
	METRIC_SUBSYSTEM_PING    = "ping"

	METRIC_CONN_TYPE_TCP = "tcp"
	METRIC_CONN_TYPE_UDP = "udp"
	METRIC_FLOW_READ     = "read"
	METRIC_FLOW_WRITE    = "write"

	EhcoAliveStateInit    = 0
	EhcoAliveStateRunning = 1
)

var (
	Hostname, _ = os.Hostname()
	ConstLabels = map[string]string{
		"ehco_runner_hostname": Hostname,
	}
)

// ping metrics
var (
	pingBuckets  = prometheus.ExponentialBuckets(0.001, 2, 12) // 1ms ~ 4s
	pingInterval = time.Second * 30

	PingResponseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   METRIC_NS,
			Subsystem:   METRIC_SUBSYSTEM_PING,
			Name:        "response_duration_seconds",
			Help:        "A histogram of latencies for ping responses.",
			Buckets:     pingBuckets,
			ConstLabels: ConstLabels,
		},
		[]string{"label", "ip"},
	)
	PingRequestTotal = prometheus.NewDesc(
		prometheus.BuildFQName(METRIC_NS, METRIC_SUBSYSTEM_PING, "requests_total"),
		"Number of ping requests sent",
		[]string{"label", "ip"},
		ConstLabels,
	)
)

// traffic metrics
var (
	EhcoAlive = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   "",
		Name:        "alive_state",
		Help:        "ehco 存活状态",
		ConstLabels: ConstLabels,
	})

	CurConnectionCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "current_connection_count",
		Help:        "当前链接数",
		ConstLabels: ConstLabels,
	}, []string{"label", "conn_type", "remote"})
	HandShakeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Namespace:   METRIC_NS,
		Name:        "handshake_duration",
		Help:        "握手时间ms",
		ConstLabels: ConstLabels,
	}, []string{"label", "conn_type", "remote"})

	NetWorkTransmitBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   METRIC_NS,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Name:        "network_transmit_bytes",
		Help:        "传输流量总量bytes",
		ConstLabels: ConstLabels,
	}, []string{"label", "conn_type", "flow", "remote"})
)

func RegisterEhcoMetrics(cfg *config.Config) error {
	// traffic
	prometheus.MustRegister(EhcoAlive)
	prometheus.MustRegister(CurConnectionCount)
	prometheus.MustRegister(NetWorkTransmitBytes)
	prometheus.MustRegister(HandShakeDuration)

	EhcoAlive.Set(EhcoAliveStateInit)

	// ping
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		prometheus.MustRegister(PingResponseDurationSeconds)
		prometheus.MustRegister(pg)
		go pg.Run()
	}
	return nil
}
