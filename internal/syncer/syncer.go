package syncer

import (
	"context"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/sampler"
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

type syncReq struct {
	Version VersionInfo         `json:"version"`
	Node    sampler.NodeMetrics `json:"node"`
	Stats   []StatsPerRule      `json:"stats"`
}

type Syncer struct {
	syncURL      string
	syncInterval time.Duration
	cmgr         cmgr.Cmgr
	sampler      *sampler.NodeSampler
	l            *zap.SugaredLogger
}

func NewSyncer(syncURL string, syncIntervalSec int, cmgr cmgr.Cmgr, sampler *sampler.NodeSampler) *Syncer {
	if syncIntervalSec <= 0 {
		syncIntervalSec = 60
	}
	return &Syncer{
		syncURL:      syncURL,
		syncInterval: time.Duration(syncIntervalSec) * time.Second,
		cmgr:         cmgr,
		sampler:      sampler,
		l:            zap.S().Named("syncer"),
	}
}

func (s *Syncer) Start(ctx context.Context) {
	s.l.Infof("Start syncer loop with interval=%s url=%s", s.syncInterval, s.syncURL)
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.l.Info("syncer stopped")
			return
		case <-ticker.C:
			if err := s.PushStats(ctx); err != nil {
				s.l.Errorf("sync failed, will retry next tick in %s: %s", s.syncInterval, err)
			}
		}
	}
}

func (s *Syncer) PushStats(ctx context.Context) error {
	drained := s.cmgr.DrainClosedConnections()
	s.l.Infof("sync once total closed connections: %d", countTotalConns(drained))

	shortCommit := constant.GitRevision
	if len(constant.GitRevision) > 7 {
		shortCommit = constant.GitRevision[:7]
	}
	req := syncReq{
		Stats:   []StatsPerRule{},
		Version: VersionInfo{Version: constant.Version, ShortCommit: shortCommit},
	}

	if s.sampler != nil {
		if nm, err := s.sampler.Sample(ctx); err != nil {
			s.l.Errorf("sample node metrics for sync: %v", err)
		} else {
			req.Node = *nm
		}
	}

	for label, conns := range drained {
		stat := StatsPerRule{RelayLabel: label}
		var totalLatency int64
		for _, c := range conns {
			stat.ConnectionCnt++
			stat.Up += c.GetStats().Up
			stat.Down += c.GetStats().Down
			totalLatency += c.GetStats().HandShakeLatency.Milliseconds()
		}
		if stat.ConnectionCnt > 0 {
			stat.HandShakeLatency = totalLatency / int64(stat.ConnectionCnt)
		}
		req.Stats = append(req.Stats, stat)
	}

	if s.syncURL == "" {
		s.l.Debugf("removed %d closed connections (no sync URL)", len(req.Stats))
		return nil
	}
	s.l.Debug("syncing data to server", zap.Any("data", req))
	return myhttp.PostJSONWithRetry(s.syncURL, &req)
}

func countTotalConns(m map[string][]conn.RelayConn) int {
	cnt := 0
	for _, v := range m {
		cnt += len(v)
	}
	return cnt
}
