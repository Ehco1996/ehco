package cmgr

import (
	"sync"

	"github.com/Ehco1996/ehco/internal/transporter"
)

// connection manager interface
type Cmgr interface {
	ListAllConnections() []transporter.RelayConn
}

type cmgrImpl struct {
	lock sync.RWMutex

	// k: relay name, v: connectionList
	connectionsMap map[string][]transporter.RelayConn
}

func NewCmgr() Cmgr {
	return &cmgrImpl{
		connectionsMap: make(map[string][]transporter.RelayConn),
	}
}

func (cm *cmgrImpl) ListAllConnections() []transporter.RelayConn {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	var conns []transporter.RelayConn
	for _, v := range cm.connectionsMap {
		conns = append(conns, v...)
	}
	return conns
}
