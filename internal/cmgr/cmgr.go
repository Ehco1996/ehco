package cmgr

import (
	"sync"

	"github.com/Ehco1996/ehco/internal/conn"
)

// connection manager interface
type Cmgr interface {
	ListAllConnections() []conn.RelayConn

	// AddConnection adds a connection to the connection manager.
	AddConnection(relayName string, conn conn.RelayConn)
}

type cmgrImpl struct {
	lock sync.RWMutex

	// k: relay name, v: connectionList
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

func (cm *cmgrImpl) AddConnection(relayName string, c conn.RelayConn) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	if _, ok := cm.connectionsMap[relayName]; !ok {
		cm.connectionsMap[relayName] = []conn.RelayConn{}
	}
	cm.connectionsMap[relayName] = append(cm.connectionsMap[relayName], c)
}
