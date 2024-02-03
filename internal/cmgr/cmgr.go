package cmgr

import (
	"sync"
)

// connection manager interface
type Cmgr interface {
	ListAllConnections() []RelayConn

	// AddConnection adds a connection to the connection manager.
	AddConnection(relayName string, conn RelayConn)
}

type cmgrImpl struct {
	lock sync.RWMutex

	// k: relay name, v: connectionList
	connectionsMap map[string][]RelayConn
}

func NewCmgr() Cmgr {
	return &cmgrImpl{
		connectionsMap: make(map[string][]RelayConn),
	}
}

func (cm *cmgrImpl) ListAllConnections() []RelayConn {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	var conns []RelayConn
	for _, v := range cm.connectionsMap {
		conns = append(conns, v...)
	}
	return conns
}

func (cm *cmgrImpl) AddConnection(relayName string, conn RelayConn) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	if _, ok := cm.connectionsMap[relayName]; !ok {
		cm.connectionsMap[relayName] = make([]RelayConn, 0)
	}
	cm.connectionsMap[relayName] = append(cm.connectionsMap[relayName], conn)
}
