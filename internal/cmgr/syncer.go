package cmgr

import (
	"context"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	myhttp "github.com/Ehco1996/ehco/pkg/http"
	"github.com/Ehco1996/ehco/pkg/metric_reader"
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

type syncReq struct {
	Version VersionInfo               `json:"version"`
	Node    metric_reader.NodeMetrics `json:"node"`
	Stats   []StatsPerRule            `json:"stats"`
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
		nm, rmm, err := cm.mr.ReadOnce(ctx)
		if err != nil {
			cm.l.Errorf("read metrics failed: %v", err)
		} else {
			req.Node = *nm
			if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
				cm.l.Errorf("add metrics to store failed: %v", err)
			}
			for _, rm := range rmm {
				if err := cm.ms.AddRuleMetric(ctx, rm); err != nil {
					cm.l.Errorf("add rule metrics to store failed: %v", err)
				}
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
