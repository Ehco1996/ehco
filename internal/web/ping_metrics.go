package web

import (
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	namespace      = "ping"
	subsystem      = ""
	pingLabelNames = []string{"ip", "host", "label"}
	pingBuckets    = []float64{
		float64(5e-05),
		float64(0.0001),
		float64(0.0002),
		float64(0.0004),
		float64(0.0008),
		float64(0.0016),
		float64(0.0032),
		float64(0.0064),
		float64(0.0128),
		float64(0.0256),
		float64(0.0512),
		float64(0.1024),
		float64(0.2048),
		float64(0.4096),
		float64(0.8192),
		float64(1.6384),
		float64(3.2768),
		float64(6.5536),
		float64(13.1072),
		float64(26.2144),
	}
	pingInerval = time.Second * 30

	PingResponseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "response_duration_seconds",
			Help:        "A histogram of latencies for ping responses.",
			Buckets:     pingBuckets,
			ConstLabels: ConstLabels,
		},
		pingLabelNames,
	)
	PingRequestTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "requests_total"),
		"Number of ping requests sent",
		pingLabelNames,
		ConstLabels,
	)
)

type PingGroup struct {
	Pingers  []*ping.Pinger
	LabelMap map[string]string
}

func initPinger(host string) *ping.Pinger {
	pinger := ping.New(host)
	if err := pinger.Resolve(); err != nil {
		logger.Errorf("[ping] failed to resolve pinger: %s\n", err.Error())
		return nil
	}
	logger.Infof("[ping] Resolved %s as %s", host, pinger.IPAddr())
	pinger.Interval = pingInerval
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

	for _, realyCfg := range cfg.Configs {
		// NOTE (https/ws/wss)://xxx.com -> xxx.com
		for _, host := range realyCfg.TCPRemotes {
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
			labelMap[host] = realyCfg.Label
		}
	}

	pingers := make([]*ping.Pinger, len(seen))
	i := 0
	for _, pinger := range seen {
		pinger.OnRecv = func(pkt *ping.Packet) {
			PingResponseDurationSeconds.WithLabelValues(
				pkt.IPAddr.String(), pkt.Addr, labelMap[pkt.Addr]).Observe(pkt.Rtt.Seconds())
			logger.Infof("[ping] %d bytes from %s: icmp_seq=%d time=%v ttl=%v",
				pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
			logger.Infof("[ping] %d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)",
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
	splay := time.Duration(pingInerval.Nanoseconds() / int64(len(pg.Pingers)))
	logger.Infof("[ping] Waiting %s between starting pingers", splay)
	for idx := range pg.Pingers {
		go func() {
			pinger := pg.Pingers[idx]
			if err := pinger.Run(); err != nil {
				logger.Infof("[ping] Starting prober err: %s", err)
			}
			logger.Infof("[ping] Starting prober for %s", pinger.Addr())
		}()
		time.Sleep(splay)
	}
}
