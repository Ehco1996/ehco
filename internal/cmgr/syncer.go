package cmgr

import (
	"context"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	myhttp "github.com/Ehco1996/ehco/pkg/http"
	"go.uber.org/zap"
)

type StatsPerRule struct {
	RelayLabel string `json:"relay_label"`

	Up               int64 `json:"up_bytes"`
	Down             int64 `json:"down_bytes"`
	ConnectionCnt    int   `json:"connection_count"`
	HandShakeLatency int64 `json:"latency_in_ms"`
}

type VersionInfo struct {
	Version     string `json:"version"`
	ShortCommit string `json:"short_commit"`
}

type syncNodeMetrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`
	NetworkOut  float64 `json:"network_out"`
}

type syncReq struct {
	Version VersionInfo     `json:"version"`
	Node    syncNodeMetrics `json:"node"`
	Stats   []StatsPerRule  `json:"stats"`
}

func (cm *cmgrImpl) syncOnce(ctx context.Context) error {
	cm.l.Infof("sync once total closed connections: %d", cm.countClosedConnection())
	// todo: opt lock
	cm.lock.Lock()

	shortCommit := constant.GitRevision
	if len(constant.GitRevision) > 7 {
		shortCommit = constant.GitRevision[:7]
	}
	req := syncReq{
		Stats:   []StatsPerRule{},
		Version: VersionInfo{Version: constant.Version, ShortCommit: shortCommit},
	}

	if cm.cfg.NeedMetrics() {
		nm, rules := metrics.Snapshot()
		req.Node = syncNodeMetrics{
			Timestamp:   nm.SyncTime.Unix(),
			CPUUsage:    nm.CPUUsage,
			MemoryUsage: nm.MemoryUsage,
			DiskUsage:   nm.DiskUsage,
			NetworkIn:   nm.NetworkIn,
			NetworkOut:  nm.NetworkOut,
		}
		if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
			cm.l.Errorf("add node metric: %v", err)
		}
		for _, rs := range rules {
			if err := cm.ms.AddRuleMetric(ctx, rs); err != nil {
				cm.l.Errorf("add rule metric: %v", err)
			}
		}
	}

	for label, conns := range cm.closedConnectionsMap {
		s := StatsPerRule{
			RelayLabel: label,
		}
		var totalLatency int64
		for _, c := range conns {
			s.ConnectionCnt++
			s.Up += c.GetStats().Up
			s.Down += c.GetStats().Down
			totalLatency += c.GetStats().HandShakeLatency.Milliseconds()
		}
		if s.ConnectionCnt > 0 {
			s.HandShakeLatency = totalLatency / int64(s.ConnectionCnt)
		}
		req.Stats = append(req.Stats, s)
	}
	cm.closedConnectionsMap = make(map[string][]conn.RelayConn)
	cm.lock.Unlock()

	if cm.cfg.NeedSync() {
		cm.l.Debug("syncing data to server", zap.Any("data", req))
		return myhttp.PostJSONWithRetry(cm.cfg.SyncURL, &req)
	} else {
		cm.l.Debugf("remove %d closed connections", len(req.Stats))
	}
	return nil
}
