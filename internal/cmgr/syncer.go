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

// sampleMetrics reads /metrics/ once and persists node + per-rule rows
// to the local store. Cheap; called on every fast tick so the dashboard
// has sub-minute resolution regardless of whether control-plane sync is
// configured.
func (cm *cmgrImpl) sampleMetrics(ctx context.Context) {
	if !cm.cfg.NeedMetrics() {
		return
	}
	nm, rmm, err := cm.mr.ReadOnce(ctx)
	if err != nil {
		cm.l.Debugf("metrics sample failed: %v", err)
		return
	}
	if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
		cm.l.Errorf("persist node metric: %v", err)
	}
	for _, rm := range rmm {
		if err := cm.ms.AddRuleMetric(ctx, rm); err != nil {
			cm.l.Errorf("persist rule metric: %v", err)
		}
	}
}

// pushStats drains closedConnectionsMap and POSTs accumulated traffic
// stats to the control plane. Called at SyncInterval cadence (default
// 60s); a tighter cadence would just spam the control plane.
func (cm *cmgrImpl) pushStats(ctx context.Context) error {
	cm.l.Infof("sync once total closed connections: %d", cm.countClosedConnection())
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
		if nm, _, err := cm.mr.ReadOnce(ctx); err != nil {
			cm.l.Errorf("read metrics for sync: %v", err)
		} else {
			req.Node = *nm
		}
	}

	for label, conns := range cm.closedConnectionsMap {
		s := StatsPerRule{RelayLabel: label}
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

	if !cm.cfg.NeedSync() {
		cm.l.Debugf("removed %d closed connections", len(req.Stats))
		return nil
	}
	cm.l.Debug("syncing data to server", zap.Any("data", req))
	return myhttp.PostJSONWithRetry(cm.cfg.SyncURL, &req)
}

type syncReq struct {
	Version VersionInfo               `json:"version"`
	Node    metric_reader.NodeMetrics `json:"node"`
	Stats   []StatsPerRule            `json:"stats"`
}
