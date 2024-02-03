package metrics

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"github.com/prometheus/node_exporter/collector"
	"go.uber.org/zap"
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

type PingGroup struct {
	Pingers  []*ping.Pinger
	LabelMap map[string]string
}

func initPinger(host string) *ping.Pinger {
	pinger := ping.New(host)
	if err := pinger.Resolve(); err != nil {
		zap.S().Named("web").Errorf("failed to resolve pinger host:%s err:%s\n", host, err.Error())
		return nil
	}
	zap.S().Named("web").Infof("Resolved %s as %s", host, pinger.IPAddr())
	pinger.Interval = pingInterval
	pinger.Timeout = time.Duration(math.MaxInt64)
	pinger.RecordRtts = false
	if runtime.GOOS != "darwin" {
		pinger.SetPrivileged(true)
	}
	return pinger
}

func NewPingGroup(cfg *config.Config) *PingGroup {
	seen := make(map[string]*ping.Pinger)
	labelMap := make(map[string]string)

	for _, relayCfg := range cfg.RelayConfigs {
		// NOTE (https/ws/wss)://xxx.com -> xxx.com
		for _, host := range relayCfg.TCPRemotes {
			if strings.Contains(host, "//") {
				host = strings.Split(host, "//")[1]
			}
			// NOTE xxx:1234 -> xxx
			if strings.Contains(host, ":") {
				host = strings.Split(host, ":")[0]
			}
			if _, ok := seen[host]; ok {
				continue
			}
			seen[host] = initPinger(host)
			labelMap[host] = relayCfg.Label
		}
	}

	pingers := make([]*ping.Pinger, len(seen))
	i := 0
	for _, pinger := range seen {
		pinger.OnRecv = func(pkt *ping.Packet) {
			PingResponseDurationSeconds.WithLabelValues(
				pkt.IPAddr.String(), pkt.Addr, labelMap[pkt.Addr]).Observe(pkt.Rtt.Seconds())
			zap.S().Named("web").Infof("%d bytes from %s: icmp_seq=%d time=%v ttl=%v",
				pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
			zap.S().Named("web").Infof("%d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)",
				pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pingers[i] = pinger
		i += 1
	}
	return &PingGroup{
		Pingers:  pingers,
		LabelMap: labelMap,
	}
}

func (pg *PingGroup) Describe(ch chan<- *prometheus.Desc) {
	ch <- PingRequestTotal
}

func (pg *PingGroup) Collect(ch chan<- prometheus.Metric) {
	for _, pinger := range pg.Pingers {
		stats := pinger.Statistics()
		ch <- prometheus.MustNewConstMetric(
			PingRequestTotal,
			prometheus.CounterValue,
			float64(stats.PacketsSent),
			stats.IPAddr.String(),
			stats.Addr,
			pg.LabelMap[stats.Addr],
		)
	}
}

func (pg *PingGroup) Run() {
	if len(pg.Pingers) <= 0 {
		return
	}
	splay := time.Duration(pingInterval.Nanoseconds() / int64(len(pg.Pingers)))
	zap.S().Named("web").Infof("Waiting %s between starting pingers", splay)
	for idx := range pg.Pingers {
		go func() {
			pinger := pg.Pingers[idx]
			if err := pinger.Run(); err != nil {
				zap.S().Named("web").Infof("Starting prober err: %s", err)
			}
			zap.S().Named("web").Infof("Starting prober for %s", pinger.Addr())
		}()
		time.Sleep(splay)
	}
}

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

func RegisterNodeExporterMetrics(cfg *config.Config) error {
	level := &promlog.AllowedLevel{}
	// mute node_exporter logger
	if err := level.Set("error"); err != nil {
		return err
	}
	promlogConfig := &promlog.Config{Level: level}
	logger := promlog.New(promlogConfig)
	// see this https://github.com/prometheus/node_exporter/pull/2463
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		return err
	}
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %s", err)
	}
	// nc.Collectors = collectors
	prometheus.MustRegister(
		nc,
		version.NewCollector("node_exporter"),
	)
	return nil
}
