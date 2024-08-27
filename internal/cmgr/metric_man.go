package cmgr

import (
	"context"
	"sync"
	"time"

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

type MetricsStore struct {
	mutex sync.RWMutex

	metrics []metric_reader.NodeMetrics

	bufSize       int
	clearDuration time.Duration
}

func NewMetricsStore(bufSize int, clearDuration time.Duration) *MetricsStore {
	return &MetricsStore{
		metrics:       make([]metric_reader.NodeMetrics, bufSize),
		clearDuration: clearDuration,
		bufSize:       bufSize,
	}
}

func (ms *MetricsStore) Add(m *metric_reader.NodeMetrics) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// 直接添加新的 metric，假设它是最新的
	ms.metrics = append(ms.metrics, *m)

	// 清理旧数据
	cutoffTime := time.Now().Add(-ms.clearDuration)
	for i, metric := range ms.metrics {
		if metric.SyncTime.After(cutoffTime) {
			ms.metrics = ms.metrics[i:]
			break
		}
	}
}

func (ms *MetricsStore) Query(startTime, endTime time.Time) []metric_reader.NodeMetrics {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	var result []metric_reader.NodeMetrics
	for i := len(ms.metrics) - 1; i >= 0; i-- {
		if ms.metrics[i].SyncTime.Before(startTime) {
			break
		}
		if !ms.metrics[i].SyncTime.After(endTime) {
			result = append(result, ms.metrics[i])
		}
	}

	// 反转结果，使其按时间升序排列
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

type QueryNodeMetricsReq struct {
	TimeRange string `json:"time_range"` // 15min/30min/1h/6h/12h/24h
	Latest    bool   `json:"latest"`     // whether to refresh the cache and get the latest data
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
		metrics, err := cm.mr.ReadOnce(ctx)
		if err != nil {
			cm.l.Errorf("read metrics failed: %v", err)
		} else {
			req.Node = *metrics
			cm.ms.Add(metrics)
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

func getTimeRangeDuration(timeRange string) time.Duration {
	switch timeRange {
	case "15min":
		return 15 * time.Minute
	case "30min":
		return 30 * time.Minute
	case "1h":
		return 1 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "24h":
		return 24 * time.Hour
	default:
		return 15 * time.Minute
	}
}

func (cm *cmgrImpl) QueryNodeMetrics(ctx context.Context, req *QueryNodeMetricsReq) ([]metric_reader.NodeMetrics, error) {
	if req.Latest {
		m, err := cm.mr.ReadOnce(ctx)
		if err != nil {
			return nil, err
		}
		cm.ms.Add(m)
		return []metric_reader.NodeMetrics{*m}, nil
	}

	startTime := time.Now().Add(-getTimeRangeDuration(req.TimeRange))
	return cm.ms.Query(startTime, time.Now()), nil
}
