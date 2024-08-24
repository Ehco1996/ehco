package metrics

import (
	"fmt"
	"math"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func (pg *PingGroup) newPinger(addr string) (*ping.Pinger, error) {
	pinger := ping.New(addr)
	if err := pinger.Resolve(); err != nil {
		pg.logger.Error("failed to resolve pinger", zap.String("addr", addr), zap.Error(err))
		return nil, err
	}
	pinger.Interval = pingInterval
	pinger.Timeout = time.Duration(math.MaxInt64)
	pinger.RecordRtts = false
	if runtime.GOOS != "darwin" {
		pinger.SetPrivileged(true)
	}
	return pinger, nil
}

type PingGroup struct {
	logger *zap.Logger

	// k: addr
	Pingers map[string]*ping.Pinger

	// k: addr v:relay rule label joined by ","
	PingerLabels map[string]string
}

func extractHost(input string) (string, error) {
	// Check if the input string has a scheme, if not, add "http://"
	if !strings.Contains(input, "://") {
		input = "http://" + input
	}
	// Parse the URL
	u, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}

func NewPingGroup(cfg *config.Config) *PingGroup {
	logger := zap.L().Named("pinger")

	pg := &PingGroup{
		logger:       logger,
		Pingers:      make(map[string]*ping.Pinger),
		PingerLabels: map[string]string{},
	}

	// parse addr from rule
	for _, relayCfg := range cfg.RelayConfigs {
		// NOTE for (https/ws/wss)://xxx.com -> xxx.com
		for _, remote := range relayCfg.Remotes {
			addr, err := extractHost(remote)
			if err != nil {
				pg.logger.Error("try parse host error", zap.Error(err))
			}
			if _, ok := pg.Pingers[addr]; ok {
				// append rule label when remote host is same
				pg.PingerLabels[addr] += fmt.Sprintf(",%s", relayCfg.Label)
				continue
			}
			if pinger, err := pg.newPinger(addr); err != nil {
				pg.logger.Error("new pinger meet error", zap.Error(err))
			} else {
				pg.Pingers[pinger.Addr()] = pinger
				pg.PingerLabels[addr] = relayCfg.Label
			}
		}
	}

	// update metrics
	for addr, pinger := range pg.Pingers {
		pinger.OnRecv = func(pkt *ping.Packet) {
			PingResponseDurationSeconds.WithLabelValues(
				pkt.IPAddr.String(), pkt.Addr, pg.PingerLabels[addr]).Observe(pkt.Rtt.Seconds())
			pg.logger.Sugar().Infof("%d bytes from %s icmp_seq=%d time=%v ttl=%v",
				pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
		pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
			pg.logger.Sugar().Infof("%d bytes from %s icmp_seq=%d time=%v ttl=%v (DUP!)",
				pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		}
	}
	return pg
}

func (pg *PingGroup) Describe(ch chan<- *prometheus.Desc) {
	ch <- PingRequestTotal
}

func (pg *PingGroup) Collect(ch chan<- prometheus.Metric) {
	for addr, pinger := range pg.Pingers {
		stats := pinger.Statistics()
		ch <- prometheus.MustNewConstMetric(
			PingRequestTotal,
			prometheus.CounterValue,
			float64(stats.PacketsSent),
			stats.IPAddr.String(),
			stats.Addr,
			pg.PingerLabels[addr],
		)
	}
}

func (pg *PingGroup) Run() {
	if len(pg.Pingers) <= 0 {
		return
	}
	pg.logger.Sugar().Infof("Start Ping Group now total pinger: %d", len(pg.Pingers))
	splay := time.Duration(pingInterval.Nanoseconds() / int64(len(pg.Pingers)))
	for addr, pinger := range pg.Pingers {
		go func() {
			if err := pinger.Run(); err != nil {
				pg.logger.Error("Starting pinger meet err", zap.String("addr", addr), zap.Error(err))
			}
		}()
		time.Sleep(splay)
	}
}
