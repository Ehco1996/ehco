package cmgr

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/pkg/metric_reader"
	"go.uber.org/zap"
)

const (
	ConnectionTypeActive = "active"
	ConnectionTypeClosed = "closed"
)

// connection manager interface/
// TODO support closed connection
type Cmgr interface {
	ListConnections(connType string, page, pageSize int) []conn.RelayConn

	// AddConnection adds a connection to the connection manager.
	AddConnection(conn conn.RelayConn)

	// RemoveConnection removes a connection from the connection manager.
	RemoveConnection(conn conn.RelayConn)

	// CountConnection returns the number of active connections.
	CountConnection(connType string) int

	GetActiveConnectCntByRelayLabel(label string) int

	// Start starts the connection manager.
	Start(ctx context.Context, errCH chan error)

	// Metrics related
	QueryNodeMetrics(ctx context.Context, req *ms.QueryNodeMetricsReq) (*ms.QueryNodeMetricsResp, error)
	QueryRuleMetrics(ctx context.Context, req *ms.QueryRuleMetricsReq) (*ms.QueryRuleMetricsResp, error)
}

type cmgrImpl struct {
	lock sync.RWMutex
	cfg  *Config
	l    *zap.SugaredLogger

	// k: relay label, v: connection list
	activeConnectionsMap map[string][]conn.RelayConn
	closedConnectionsMap map[string][]conn.RelayConn

	ms *ms.MetricsStore
	mr metric_reader.Reader
}

func NewCmgr(cfg *Config) (Cmgr, error) {
	cmgr := &cmgrImpl{
		cfg:                  cfg,
		l:                    zap.S().Named("cmgr"),
		activeConnectionsMap: make(map[string][]conn.RelayConn),
		closedConnectionsMap: make(map[string][]conn.RelayConn),
	}
	if cfg.NeedMetrics() {
		cmgr.mr = metric_reader.NewReader(cfg.MetricsURL, cfg.ApiToken)

		homeDir, _ := os.UserHomeDir()
		dbPath := filepath.Join(homeDir, ".ehco", "metrics.db")
		ms, err := ms.NewMetricsStore(dbPath)
		if err != nil {
			return nil, err
		}
		cmgr.ms = ms
	}
	return cmgr, nil
}

func (cm *cmgrImpl) ListConnections(connType string, page, pageSize int) []conn.RelayConn {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	var total int
	var m map[string][]conn.RelayConn

	if connType == ConnectionTypeActive {
		total = cm.countActiveConnection()
		m = cm.activeConnectionsMap
	} else {
		total = cm.countClosedConnection()
		m = cm.closedConnectionsMap

	}

	start := (page - 1) * pageSize
	if start > total {
		return []conn.RelayConn{} // Return empty slice if start index is more than length
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	relayLabelList := make([]string, 0, len(m))
	for k := range m {
		relayLabelList = append(relayLabelList, k)
	}
	// Sort the relay label list to make the result more predictable
	sort.Strings(relayLabelList)

	var conns []conn.RelayConn
	for _, label := range relayLabelList {
		conns = append(conns, m[label]...)
	}
	if end > len(conns) {
		end = len(conns) // Don't let the end index be more than slice length
	}
	return conns[start:end]
}

func (cm *cmgrImpl) AddConnection(c conn.RelayConn) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	label := c.GetRelayLabel()

	if _, ok := cm.activeConnectionsMap[label]; !ok {
		cm.activeConnectionsMap[label] = []conn.RelayConn{}
	}
	cm.activeConnectionsMap[label] = append(cm.activeConnectionsMap[label], c)
}

func (cm *cmgrImpl) RemoveConnection(c conn.RelayConn) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	label := c.GetRelayLabel()
	connections, ok := cm.activeConnectionsMap[label]
	if !ok {
		return // If the label doesn't exist, nothing to remove
	}

	// Find and remove the connection from activeConnectionsMap
	for i, activeConn := range connections {
		if activeConn == c {
			cm.activeConnectionsMap[label] = append(connections[:i], connections[i+1:]...)
			break
		}
	}
	// Add to closedConnectionsMap
	cm.closedConnectionsMap[label] = append(cm.closedConnectionsMap[label], c)
}

func (cm *cmgrImpl) CountConnection(connType string) int {
	if connType == ConnectionTypeActive {
		return cm.countActiveConnection()
	} else {
		return cm.countClosedConnection()
	}
}

func (cm *cmgrImpl) countActiveConnection() int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	cnt := 0
	for _, v := range cm.activeConnectionsMap {
		cnt += len(v)
	}
	return cnt
}

func (cm *cmgrImpl) countClosedConnection() int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	cnt := 0
	for _, v := range cm.closedConnectionsMap {
		cnt += len(v)
	}
	return cnt
}

func (cm *cmgrImpl) GetActiveConnectCntByRelayLabel(label string) int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return len(cm.activeConnectionsMap[label])
}

// metricsSampleInterval is the cadence at which we read /metrics/ and
// persist a row to the local store, so the dashboard's Node page has
// sub-minute resolution. SyncInterval (default 60s) controls the coarser
// control-plane push only.
const metricsSampleInterval = 5 * time.Second

func (cm *cmgrImpl) Start(ctx context.Context, errCH chan error) {
	cm.l.Infof("Start Cmgr sync interval=%d sample interval=%s", cm.cfg.SyncInterval, metricsSampleInterval)
	syncEvery := int(time.Duration(cm.cfg.SyncInterval)*time.Second/metricsSampleInterval) - 1
	if syncEvery < 1 {
		syncEvery = 1
	}
	ticker := time.NewTicker(metricsSampleInterval)
	defer ticker.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			cm.l.Info("sync stop")
			return
		case <-ticker.C:
			cm.sampleMetrics(ctx)
			tick++
			if tick%syncEvery != 0 {
				continue
			}
			// Tolerate transient sync failures: retryablehttp already does
			// internal backoff; on final error we just log and wait for the
			// next tick. The traffic stats accumulated for this interval are
			// dropped on the floor.
			// TODO: persist unsent stats locally so they can be retried on
			// later ticks instead of being lost when the upstream is down.
			if err := cm.pushStats(ctx); err != nil {
				cm.l.Errorf("sync failed, will retry next tick in %ds: %s", cm.cfg.SyncInterval, err)
			}
		}
	}
}

func (cm *cmgrImpl) QueryNodeMetrics(ctx context.Context, req *ms.QueryNodeMetricsReq) (*ms.QueryNodeMetricsResp, error) {
	return cm.ms.QueryNodeMetric(ctx, req)
}

func (cm *cmgrImpl) QueryRuleMetrics(ctx context.Context, req *ms.QueryRuleMetricsReq) (*ms.QueryRuleMetricsResp, error) {
	return cm.ms.QueryRuleMetric(ctx, req)
}
