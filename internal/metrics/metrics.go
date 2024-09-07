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

	// 1ms ~ 5s (1ms 到 437ms )
	msBuckets = prometheus.ExponentialBuckets(1, 1.5, 16)
)

// ping metrics
var (
	pingInterval                     = time.Second * 30
	PingResponseDurationMilliseconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   METRIC_NS,
			Subsystem:   METRIC_SUBSYSTEM_PING,
			Name:        "response_duration_milliseconds",
			Help:        "A histogram of latencies for ping responses.",
			Buckets:     msBuckets,
			ConstLabels: ConstLabels,
		},
		[]string{"label", "remote", "ip"},
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

	HandShakeDurationMilliseconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Buckets:     msBuckets,
		Subsystem:   METRIC_SUBSYSTEM_TRAFFIC,
		Namespace:   METRIC_NS,
		Name:        "handshake_duration_milliseconds",
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
	prometheus.MustRegister(HandShakeDurationMilliseconds)

	EhcoAlive.Set(EhcoAliveStateInit)

	// ping
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		prometheus.MustRegister(PingResponseDurationMilliseconds)
		go pg.Run()
	}
	return nil
}
