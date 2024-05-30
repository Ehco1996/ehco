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

	METRIC_LABEL_REMOTE = "remote"

	METRIC_LABEL_CONN_FLOW = "flow"
	METRIC_CONN_FLOW_WRITE = "write"
	METRIC_CONN_FLOW_READ  = "read"

	METRIC_LABEL_CONN_TYPE = "type"
	METRIC_CONN_TYPE_TCP   = "tcp"
	METRIC_CONN_TYPE_UDP   = "udp"

	EhcoAliveStateInit    = 0
	EhcoAliveStateRunning = 1
)

// ping metrics
var (
	pingLabelNames = []string{"ip", "host", "label"}
	pingBuckets    = prometheus.ExponentialBuckets(0.001, 2, 12) // 1ms ~ 4s
	pingInterval   = time.Second * 30

	PingResponseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   METRIC_NS,
			Subsystem:   METRIC_SUBSYSTEM_PING,
			Name:        "response_duration_seconds",
			Help:        "A histogram of latencies for ping responses.",
			Buckets:     pingBuckets,
			ConstLabels: ConstLabels,
		},
		pingLabelNames,
	)
	PingRequestTotal = prometheus.NewDesc(
		prometheus.BuildFQName(METRIC_NS, METRIC_SUBSYSTEM_PING, "requests_total"),
		"Number of ping requests sent",
		pingLabelNames,
		ConstLabels,
	)
)

// traffic metrics
var (
	Hostname, _ = os.Hostname()

	ConstLabels = map[string]string{
		"ehco_runner_hostname": Hostname,
	}

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
