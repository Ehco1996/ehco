package web

import (
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	pingLabelNames = []string{"ip", "host"}
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
	pingInerval = time.Second * 15

	pingResponseTtl = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "ping",
			Name:        "response_ttl",
			Help:        "The last response Time To Live (TTL).",
			ConstLabels: ConstLabels,
		},
		pingLabelNames,
	)
	pingResponseDuplicates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   "ping",
			Name:        "response_duplicates_total",
			Help:        "The number of duplicated response packets.",
			ConstLabels: ConstLabels,
		},
		pingLabelNames,
	)
	PingResponseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "ping",
			Name:        "response_duration_seconds",
			Help:        "A histogram of latencies for ping responses.",
			Buckets:     pingBuckets,
			ConstLabels: ConstLabels,
		},
		pingLabelNames,
	)
)

type SmokepingCollector struct {
	pg           *PingGroup
	requestsSent *prometheus.Desc
}

func NewSmokepingCollector(pg *PingGroup, pingResponseSeconds prometheus.HistogramVec) *SmokepingCollector {
	for _, pinger := range pg.Pingers {
		// Init all metrics to 0s.
		ipAddr := pinger.IPAddr().String()
		pingResponseDuplicates.WithLabelValues(ipAddr, pinger.Addr())
		pingResponseSeconds.WithLabelValues(ipAddr, pinger.Addr())
		pingResponseTtl.WithLabelValues(ipAddr, pinger.Addr())
		// Setup handler functions.
		pinger.OnRecv = func(pkt *ping.Packet) {
			pingResponseSeconds.WithLabelValues(pkt.IPAddr.String(), pkt.Addr).Observe(pkt.Rtt.Seconds())
			pingResponseTtl.WithLabelValues(pkt.IPAddr.String(), pkt.Addr).Set(float64(pkt.Ttl))
			logger.Infof("[ping] %d bytes from %s: icmp_seq=%d time=%v ttl=%v",
				pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
			pingResponseDuplicates.WithLabelValues(pkt.IPAddr.String(), pkt.Addr).Inc()
			logger.Infof("[ping] %d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)",
				pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
	}
	return &SmokepingCollector{
		pg: pg,
		requestsSent: prometheus.NewDesc(
			prometheus.BuildFQName("ping", "", "requests_total"),
			"Number of ping requests sent",
			pingLabelNames,
			ConstLabels,
		),
	}
}

func (s *SmokepingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.requestsSent
}

func (s *SmokepingCollector) Collect(ch chan<- prometheus.Metric) {
	for _, pinger := range s.pg.Pingers {
		stats := pinger.Statistics()
		ch <- prometheus.MustNewConstMetric(
			s.requestsSent,
			prometheus.CounterValue,
			float64(stats.PacketsSent),
			stats.IPAddr.String(),
			stats.Addr,
		)
	}
}

type PingGroup struct {
	Pingers []*ping.Pinger
	Hosts   []string
}

func NewPingGroup(hosts []string) *PingGroup {
	seen := make(map[string]*ping.Pinger)
	for _, host := range hosts {
		// NOTE (https/ws/wss)://xxx.com -> xxx.com
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
		pinger := ping.New(host)
		if err := pinger.Resolve(); err != nil {
			logger.Errorf("[ping] failed to resolve pinger: %s\n", err.Error())
			continue
		}
		logger.Infof("[ping] Resolved %s as %s", host, pinger.IPAddr())

		pinger.Interval = pingInerval
		pinger.Timeout = time.Duration(math.MaxInt64)
		pinger.RecordRtts = false
		if runtime.GOOS != "darwin" {
			pinger.SetPrivileged(true)
		}
		seen[host] = pinger
	}
	pingers := make([]*ping.Pinger, len(seen))
	i := 0
	for _, p := range seen {
		pingers[i] = p
		i += 1
	}
	return &PingGroup{
		Pingers: pingers,
		Hosts:   hosts,
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
