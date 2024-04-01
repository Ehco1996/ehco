package cmgr

import (
	"net/http"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	myhttp "github.com/Ehco1996/ehco/pkg/http"
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
	Version VersionInfo    `json:"version"`
	Stats   []StatsPerRule `json:"stats"`
}

func (cm *cmgrImpl) syncOnce() error {
	cm.l.Infof("sync once total closed connections: %d", cm.countClosedConnection())
	// todo: opt lock
	cm.lock.Lock()

	shorCommit := constant.GitRevision
	if len(constant.GitRevision) > 7 {
		shorCommit = constant.GitRevision[:7]
	}
	req := syncReq{
		Stats:   []StatsPerRule{},
		Version: VersionInfo{Version: constant.Version, ShortCommit: shorCommit},
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
		return myhttp.PostJson(http.DefaultClient, cm.cfg.SyncURL, &req)
	} else {
		cm.l.Debugf("remove %d closed connections", len(req.Stats))
	}
	return nil
}
