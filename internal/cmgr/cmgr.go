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
	QueryNodeMetrics(ctx context.Context, req *ms.QueryNodeMetricsReq, refresh bool) (*ms.QueryNodeMetricsResp, error)
	QueryRuleMetrics(ctx context.Context, req *ms.QueryRuleMetricsReq, refresh bool) (*ms.QueryRuleMetricsResp, error)
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
		cmgr.mr = metric_reader.NewReader(cfg.MetricsURL)

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

func (cm *cmgrImpl) Start(ctx context.Context, errCH chan error) {
	cm.l.Infof("Start Cmgr sync interval=%d", cm.cfg.SyncInterval)
	ticker := time.NewTicker(time.Second * time.Duration(cm.cfg.SyncInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cm.l.Info("sync stop")
			return
		case <-ticker.C:
			if err := cm.syncOnce(ctx); err != nil {
				cm.l.Errorf("meet non retry error: %s ,exit now", err)
				errCH <- err
			}
		}
	}
}

func (cm *cmgrImpl) QueryNodeMetrics(ctx context.Context, req *ms.QueryNodeMetricsReq, refresh bool) (*ms.QueryNodeMetricsResp, error) {
	if refresh {
		nm, _, err := cm.mr.ReadOnce(ctx)
		if err != nil {
			return nil, err
		}
		if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
			return nil, err
		}
	}
	return cm.ms.QueryNodeMetric(ctx, req)
}

func (cm *cmgrImpl) QueryRuleMetrics(ctx context.Context, req *ms.QueryRuleMetricsReq, refresh bool) (*ms.QueryRuleMetricsResp, error) {
	if refresh {
		_, rm, err := cm.mr.ReadOnce(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range rm {
			if err := cm.ms.AddRuleMetric(ctx, m); err != nil {
				return nil, err
			}
		}
	}
	return cm.ms.QueryRuleMetric(ctx, req)
}
