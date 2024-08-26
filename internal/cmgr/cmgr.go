package cmgr

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/pkg/metric_reader"
	"go.uber.org/zap"
)

const (
	ConnectionTypeActive = "active"
	ConnectionTypeClosed = "closed"
)

type QueryNodeMetricsReq struct {
	TimeRange string `json:"time_range"` // 15min/30min/1h/6h/12h/24h
	Num       int    `json:"num"`        // number of nodes to query
}

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

	QueryNodeMetrics(ctx context.Context, req *QueryNodeMetricsReq) ([]metric_reader.NodeMetrics, error)
}

type cmgrImpl struct {
	lock sync.RWMutex
	cfg  *Config
	l    *zap.SugaredLogger

	// k: relay label, v: connection list
	activeConnectionsMap map[string][]conn.RelayConn
	closedConnectionsMap map[string][]conn.RelayConn

	mr metric_reader.Reader
	ms []*metric_reader.NodeMetrics // TODO gc this
}

func NewCmgr(cfg *Config) Cmgr {
	cmgr := &cmgrImpl{
		cfg:                  cfg,
		l:                    zap.S().Named("cmgr"),
		activeConnectionsMap: make(map[string][]conn.RelayConn),
		closedConnectionsMap: make(map[string][]conn.RelayConn),
	}
	if cfg.NeedMetrics() {
		cmgr.mr = metric_reader.NewReader(cfg.MetricsURL)
	}
	return cmgr
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
	// sync once at the beginning
	if err := cm.syncOnce(ctx); err != nil {
		cm.l.Errorf("meet non retry error: %s ,exit now", err)
		errCH <- err
		return
	}

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

func (cm *cmgrImpl) QueryNodeMetrics(ctx context.Context, req *QueryNodeMetricsReq) ([]metric_reader.NodeMetrics, error) {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	var startTime time.Time
	switch req.TimeRange {
	case "15min":
		startTime = time.Now().Add(-15 * time.Minute)
	case "30min":
		startTime = time.Now().Add(-30 * time.Minute)
	case "1h":
		startTime = time.Now().Add(-1 * time.Hour)
	case "6h":
		startTime = time.Now().Add(-6 * time.Hour)
	case "12h":
		startTime = time.Now().Add(-12 * time.Hour)
	case "24h":
		startTime = time.Now().Add(-24 * time.Hour)
	default:
		// default to 15min
		startTime = time.Now().Add(-15 * time.Minute)
	}

	res := []metric_reader.NodeMetrics{}
	for _, metrics := range cm.ms {
		if metrics.SyncTime.After(startTime) {
			res = append(res, *metrics)
		}
		if req.Num > 0 && len(res) >= req.Num {
			break
		}
	}
	return res, nil
}
