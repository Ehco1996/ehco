package cmgr

import (
	"sync"

	"github.com/Ehco1996/ehco/internal/conn"
)

// connection manager interface
type Cmgr interface {
	ListAllConnections() []conn.RelayConn

	// AddConnection adds a connection to the connection manager.
	AddConnection(conn conn.RelayConn)
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

func (cm *cmgrImpl) ListAllConnections() []conn.RelayConn {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	var conns []conn.RelayConn
	for _, v := range cm.connectionsMap {
		conns = append(conns, v...)
	}
	return conns
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
