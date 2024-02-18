package conn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/bytes"
	"go.uber.org/zap"
)

var (
	idleTimeout = 30 * time.Second
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

func (c *innerConn) setDeadline(isRead bool) {
	// set the read deadline to avoid hanging read for non-TCP connections
	// because tcp connections have closeWrite/closeRead so no need to set read deadline
	if _, ok := c.Conn.(*net.TCPConn); !ok {
		deadline := time.Now().Add(idleTimeout)
		if isRead {
			_ = c.Conn.SetReadDeadline(deadline)
		} else {
			_ = c.Conn.SetWriteDeadline(deadline)
		}
	}
}

func (c *innerConn) recordStats(n int, isRead bool) {
	if isRead {
		metrics.NetWorkTransmitBytes.WithLabelValues(
			c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_READ,
		).Add(float64(n))
		c.stats.Record(0, int64(n))
	} else {
		metrics.NetWorkTransmitBytes.WithLabelValues(
			c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_WRITE,
		).Add(float64(n))
		c.stats.Record(int64(n), 0)
	}
}

// 修改Read和Write方法以使用recordStats
func (c *innerConn) Read(p []byte) (n int, err error) {
	c.setDeadline(true)
	n, err = c.Conn.Read(p)
	c.recordStats(n, true) // true for read operation
	return
}

func (c *innerConn) Write(p []byte) (n int, err error) {
	c.setDeadline(false)
	n, err = c.Conn.Write(p)
	c.recordStats(n, false) // false for write operation
	return
}

func (c innerConn) Close() error {
	return c.Conn.Close()
}

func (c innerConn) CloseWrite() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseWrite()
	}
	return nil
}

func (c innerConn) CloseRead() error {
	if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
		return tcpConn.CloseRead()
	}
	return nil
}

func shortHashSHA256(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)[:7]
}

func connectionName(conn net.Conn) string {
	return fmt.Sprintf("l:<%s> r:<%s>", conn.LocalAddr(), conn.RemoteAddr())
}

func copyConn(conn1, conn2 *innerConn) error {
	errCH := make(chan error, 1)
	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		_, err := io.Copy(conn2, conn1)
		_ = conn2.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	_, err := io.Copy(conn1, conn2)
	_ = conn1.CloseWrite()

	err2 := <-errCH
	_ = conn1.CloseRead()
	_ = conn2.CloseRead()

	// handle errors, need to combine errors from both directions
	if err != nil && err2 != nil {
		err = fmt.Errorf("transport errors in both directions: %v, %v", err, err2)
	}
	if err != nil {
		return err
	}
	return err2
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
	defer rc.Close() // nolint: errcheck
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
