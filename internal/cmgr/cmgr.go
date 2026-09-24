package cmgr

import (
	"sync"

	"github.com/Ehco1996/ehco/internal/conn"
	"go.uber.org/zap"
)

const (
	ConnectionTypeActive = "active"
	ConnectionTypeClosed = "closed"

	maxClosedConnectionsPerLabel = 5000
)

// Cmgr manages relay connections and tracks active/closed connection states.
type Cmgr interface {
	// AddConnection adds an active connection.
	AddConnection(conn conn.RelayConn)

	// RemoveConnection removes an active connection and optionally retains it for closed stats.
	RemoveConnection(conn conn.RelayConn)

	// CountConnection returns the count of connections of the specified type.
	CountConnection(connType string) int

	// GetActiveConnectCntByRelayLabel returns active connections count for a label.
	GetActiveConnectCntByRelayLabel(label string) int

	// DrainClosedConnections returns and clears all retained closed connections.
	DrainClosedConnections() map[string][]conn.RelayConn
}

type cmgrImpl struct {
	lock sync.RWMutex
	l    *zap.SugaredLogger

	trackClosed          bool
	activeConnectionsMap map[string][]conn.RelayConn
	closedConnectionsMap map[string][]conn.RelayConn
}

func NewCmgr(trackClosed bool) Cmgr {
	return &cmgrImpl{
		l:                    zap.S().Named("cmgr"),
		trackClosed:          trackClosed,
		activeConnectionsMap: make(map[string][]conn.RelayConn),
		closedConnectionsMap: make(map[string][]conn.RelayConn),
	}
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
		return
	}

	for i, activeConn := range connections {
		if activeConn == c {
			cm.activeConnectionsMap[label] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	if cm.trackClosed {
		if len(cm.closedConnectionsMap[label]) < maxClosedConnectionsPerLabel {
			cm.closedConnectionsMap[label] = append(cm.closedConnectionsMap[label], c)
		}
	}
}

func (cm *cmgrImpl) CountConnection(connType string) int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	if connType == ConnectionTypeActive {
		cnt := 0
		for _, v := range cm.activeConnectionsMap {
			cnt += len(v)
		}
		return cnt
	}
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

func (cm *cmgrImpl) DrainClosedConnections() map[string][]conn.RelayConn {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	drained := cm.closedConnectionsMap
	cm.closedConnectionsMap = make(map[string][]conn.RelayConn)
	return drained
}
