package cmgr

import (
	"sort"
	"sync"

	"github.com/Ehco1996/ehco/internal/conn"
)

// connection manager interface
type Cmgr interface {
	ListConnections(page, pageSize int) []conn.RelayConn

	// AddConnection adds a connection to the connection manager.
	AddConnection(conn conn.RelayConn)

	// RemoveConnection removes a connection from the connection manager.
	RemoveConnection(conn conn.RelayConn)

	// CountConnection returns the number of active connections.
	CountConnection() int
}

type cmgrImpl struct {
	lock sync.RWMutex

	// k: relay label, v: connection list
	activeConnectionsMap map[string][]conn.RelayConn
	closedConnectionsMap map[string][]conn.RelayConn
}

func NewCmgr() Cmgr {
	return &cmgrImpl{
		activeConnectionsMap: make(map[string][]conn.RelayConn),
		closedConnectionsMap: make(map[string][]conn.RelayConn),
	}
}

func (cm *cmgrImpl) ListConnections(page, pageSize int) []conn.RelayConn {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	total := cm.CountConnection()

	start := (page - 1) * pageSize
	if start > total {
		return []conn.RelayConn{} // Return empty slice if start index is more than length
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	relayLabelList := make([]string, 0, len(cm.activeConnectionsMap))
	for k := range cm.activeConnectionsMap {
		relayLabelList = append(relayLabelList, k)
	}
	// Sort the relay label list to make the result more predictable
	sort.Strings(relayLabelList)

	var conns []conn.RelayConn
	for _, label := range relayLabelList {
		conns = append(conns, cm.activeConnectionsMap[label]...)
	}
	if end > len(conns) {
		end = len(conns) // Don't let the end index be more than slice length
	}
	// group by status
	sort.Slice(conns, func(i, j int) bool {
		return conns[i].GetRelayLabel() < conns[j].GetRelayLabel()
	})
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

func (cm *cmgrImpl) CountConnection() int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	cnt := 0
	for _, v := range cm.activeConnectionsMap {
		cnt += len(v)
	}
	return cnt
}
