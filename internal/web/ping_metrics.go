package web

import (
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
)

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

type PingGroup struct {
	Pingers  []*ping.Pinger
	LabelMap map[string]string
}

func initPinger(host string) *ping.Pinger {
	pinger := ping.New(host)
	if err := pinger.Resolve(); err != nil {
		L.Errorf("failed to resolve pinger host:%s err:%s\n", host, err.Error())
		return nil
	}
	L.Infof("Resolved %s as %s", host, pinger.IPAddr())
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
			L.Infof("%d bytes from %s: icmp_seq=%d time=%v ttl=%v",
				pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
			L.Infof("%d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)",
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
	L.Infof("Waiting %s between starting pingers", splay)
	for idx := range pg.Pingers {
		go func() {
			pinger := pg.Pingers[idx]
			if err := pinger.Run(); err != nil {
				L.Infof("Starting prober err: %s", err)
			}
			L.Infof("Starting prober for %s", pinger.Addr())
		}()
		time.Sleep(splay)
	}
}
