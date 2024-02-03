package conn

import (
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/transporter"
	"go.uber.org/zap"
)

type RelayConn interface {
	// Transport transports data between the client and the remote server.
	// The remoteLabel is the label of the remote server.
	Transport(remoteLabel string) error
}

func NewRelayConn(label string, clientConn, remoteConn net.Conn) RelayConn {
	return &relayConnImpl{
		Label: label,
		Stats: &Stats{},

		clientConn: clientConn,
		remoteConn: remoteConn,
	}
}

type relayConnImpl struct {
	// same with relay label
	Label string `json:"label"`
	Stats *Stats `json:"stats"`

	clientConn net.Conn
	remoteConn net.Conn
}

func (rc *relayConnImpl) Transport(remoteLabel string) error {
	name := rc.Name()
	shortName := shortHashSHA256(name)
	cl := zap.L().Named(shortName)
	cl.Debug("transport start", zap.String("full name", name), zap.String("stats", rc.Stats.String()))

	mc := &metricsConn{
		Reader:      rc.remoteConn,
		Writer:      rc.clientConn,
		remoteLabel: remoteLabel,
		stats:       rc.Stats,
	}

	sc := &metricsConn{
		Reader:      rc.remoteConn,
		Writer:      rc.clientConn,
		remoteLabel: remoteLabel,
		stats:       rc.Stats,
	}

	err := transporter.CopyConn(mc, sc)
	if err != nil {
		cl.Error("transport error", zap.Error(err))
	}
	cl.Debug("transport end", zap.String("stats", rc.Stats.String()))
	return err
}

func (rc *relayConnImpl) Name() string {
	return fmt.Sprintf("c1:[%s] c2:[%s]", connectionName(rc.clientConn), connectionName(rc.remoteConn))
}
