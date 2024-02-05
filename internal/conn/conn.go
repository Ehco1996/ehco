package conn

import (
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

type RelayConn interface {
	// Transport transports data between the client and the remote server.
	// The remoteLabel is the label of the remote server.
	Transport(remoteLabel string) error

	// GetRelayLabel returns the label of the Relay instance.
	GetRelayLabel() string

	GetStats() *Stats
}

func NewRelayConn(relayName string, clientConn, remoteConn net.Conn) RelayConn {
	return &relayConnImpl{
		RelayLabel: relayName,
		Stats:      &Stats{Up: 0, Down: 0},

		clientConn: clientConn,
		remoteConn: remoteConn,
	}
}

type relayConnImpl struct {
	RelayLabel string `json:"relay_label"`
	Closed     bool   `json:"closed"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`

	Stats *Stats `json:"stats"`

	clientConn net.Conn
	remoteConn net.Conn
}

func (rc *relayConnImpl) Transport(remoteLabel string) error {
	name := rc.Name()
	shortName := shortHashSHA256(name)
	cl := zap.L().Named(shortName)
	cl.Debug("transport start", zap.String("full name", name), zap.String("stats", rc.Stats.String()))

	c1 := &metricsConn{
		stats:          rc.Stats,
		remoteLabel:    remoteLabel,
		underlyingConn: rc.clientConn,
	}

	c2 := &metricsConn{
		stats:          rc.Stats,
		remoteLabel:    remoteLabel,
		underlyingConn: rc.remoteConn,
	}
	rc.StartTime = time.Now()
	err := CopyConn(c1, c2)
	if err != nil {
		cl.Error("transport error", zap.Error(err))
	}
	cl.Debug("transport end", zap.String("stats", rc.Stats.String()))
	rc.Closed = true
	rc.EndTime = time.Now()
	return err
}

func (rc *relayConnImpl) GetTime() string {
	if rc.EndTime.IsZero() {
		return fmt.Sprintf("%s - N/A", rc.StartTime.Format(time.Stamp))
	}
	return fmt.Sprintf("%s - %s", rc.StartTime.Format(time.Stamp), rc.EndTime.Format(time.Stamp))
}

func (rc *relayConnImpl) Name() string {
	return fmt.Sprintf("c1:[%s] c2:[%s]", connectionName(rc.clientConn), connectionName(rc.remoteConn))
}

func (rc *relayConnImpl) Flow() string {
	return fmt.Sprintf("%s <-> %s", rc.clientConn.LocalAddr(), rc.remoteConn.RemoteAddr())
}
func (rc *relayConnImpl) GetRelayLabel() string {
	return rc.RelayLabel
}

func (rc *relayConnImpl) GetStats() *Stats {
	return rc.Stats
}
