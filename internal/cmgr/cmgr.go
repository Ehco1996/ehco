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

	CountConnection() int
}

type cmgrImpl struct {
	lock sync.RWMutex

	// k: relay label, v: connection list
	connectionsMap map[string][]conn.RelayConn
}

func NewCmgr() Cmgr {
	return &cmgrImpl{
		connectionsMap: make(map[string][]conn.RelayConn),
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

	relayLabelList := make([]string, 0, len(cm.connectionsMap))
	for k := range cm.connectionsMap {
		relayLabelList = append(relayLabelList, k)
	}
	// Sort the relay label list to make the result more predictable
	sort.Strings(relayLabelList)

	var conns []conn.RelayConn
	for _, label := range relayLabelList {
		conns = append(conns, cm.connectionsMap[label]...)
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

	if _, ok := cm.connectionsMap[label]; !ok {
		cm.connectionsMap[label] = []conn.RelayConn{}
	}
	cm.connectionsMap[label] = append(cm.connectionsMap[label], c)
}

func (cm *cmgrImpl) CountConnection() int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	cnt := 0
	for _, v := range cm.connectionsMap {
		cnt += len(v)
	}
	return cnt
}
