package conn

import (
	"fmt"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/bytes"
	"go.uber.org/zap"
)

type Stats struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func (s *Stats) Record(up, down int64) {
	s.Up += up
	s.Down += down
}

func (s *Stats) String() string {
	return fmt.Sprintf("up: %s, down: %s", bytes.PrettyByteSize(float64(s.Up)), bytes.PrettyByteSize(float64(s.Down)))
}

type innerConn struct {
	net.Conn

	remoteLabel string
	stats       *Stats
}

func (c innerConn) Read(p []byte) (n int, err error) {
	n, err = c.Conn.Read(p)
	// increment the metric for the read bytes
	metrics.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	// record the traffic
	c.stats.Record(int64(n), 0)
	return
}

func (c innerConn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	metrics.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	c.stats.Record(0, int64(n))
	return
}

func (c innerConn) Close() error {
	return c.Conn.Close()
}

func (c innerConn) CloseWrite() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseWrite()
	}
	return c.Conn.Close()
}

func (c innerConn) CloseRead() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseRead()
	}
	return c.Conn.Close()
}

type RelayConn interface {
	// Transport transports data between the client and the remote server.
	// The remoteLabel is the label of the remote server.
	Transport(remoteLabel string) error

	// GetRelayLabel returns the label of the Relay instance.
	GetRelayLabel() string

	GetStats() *Stats

	Close() error
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
	defer rc.Close()
	name := rc.Name()
	shortName := fmt.Sprintf("%s-%s", rc.RelayLabel, shortHashSHA256(name))
	cl := zap.L().Named(shortName)
	cl.Debug("transport start", zap.String("full name", name), zap.String("stats", rc.Stats.String()))
	c1 := &innerConn{
		stats:       rc.Stats,
		remoteLabel: remoteLabel,
		Conn:        rc.clientConn,
	}
	c2 := &innerConn{
		stats:       rc.Stats,
		remoteLabel: remoteLabel,
		Conn:        rc.remoteConn,
	}
	rc.StartTime = time.Now().Local()
	err := copyConn(c1, c2)
	if err != nil {
		cl.Error("transport error", zap.Error(err))
	}
	cl.Debug("transport end", zap.String("stats", rc.Stats.String()))
	rc.EndTime = time.Now().Local()
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
	return fmt.Sprintf("%s <-> %s", rc.clientConn.RemoteAddr(), rc.remoteConn.RemoteAddr())
}

func (rc *relayConnImpl) GetRelayLabel() string {
	return rc.RelayLabel
}

func (rc *relayConnImpl) GetStats() *Stats {
	return rc.Stats
}

func (rc *relayConnImpl) Close() error {
	err1 := rc.clientConn.Close()
	err2 := rc.remoteConn.Close()
	rc.Closed = true
	return combineErrors(err1, err2)
}

func combineErrors(err1, err2 error) error {
	if err1 != nil && err2 != nil {
		return fmt.Errorf("combineErrors: %v, %v", err1, err2)
	}
	if err1 != nil {
		return err1
	}
	return err2
}
