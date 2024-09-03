package metrics

import (
	"math"
	"runtime"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/go-ping/ping"
	"go.uber.org/zap"
)

func (pg *PingGroup) newPinger(ruleLabel string, remote string, addr string) (*ping.Pinger, error) {
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
	pinger.OnRecv = func(pkt *ping.Packet) {
		ip := pkt.IPAddr.String()
		PingResponseDurationMilliseconds.WithLabelValues(
			ruleLabel, remote, ip).Observe(float64(pkt.Rtt.Milliseconds()))
		pg.logger.Sugar().Infof("%d bytes from %s icmp_seq=%d time=%v ttl=%v",
			pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
	}
	return pinger, nil
}

type PingGroup struct {
	logger *zap.Logger

	// k: addr
	Pingers map[string]*ping.Pinger
}

func NewPingGroup(cfg *config.Config) *PingGroup {
	pg := &PingGroup{
		logger:  zap.L().Named("pinger"),
		Pingers: make(map[string]*ping.Pinger),
	}
	for _, relayCfg := range cfg.RelayConfigs {
		for _, remote := range relayCfg.GetAllRemotes() {
			addr, err := remote.GetAddrHost()
			if err != nil {
				pg.logger.Error("try parse host error", zap.Error(err))
			}
			if pinger, err := pg.newPinger(relayCfg.Label, remote.Address, addr); err != nil {
				pg.logger.Error("new pinger meet error", zap.Error(err))
			} else {
				pg.Pingers[addr] = pinger
			}
		}
	}
	return pg
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
