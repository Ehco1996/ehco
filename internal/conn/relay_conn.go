package conn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/buffer"
	"github.com/Ehco1996/ehco/pkg/bytes"
	"go.uber.org/zap"
)

const (
	shortHashLength = 7
)

var ErrIdleTimeout = errors.New("connection closed due to idle timeout")

// RelayConn is the interface that represents a relay connection.
// it contains two connections: clientConn and remoteConn
// clientConn is the connection from the client to the relay server
// remoteConn is the connection from the relay server to the remote server
// and the main function is to transport data between the two connections
type RelayConn interface {
	// Transport transports data between the client and the remote connection.
	Transport() error
	GetRelayLabel() string
	GetStats() *Stats
	Close() error
}

type RelayConnOption func(*relayConnImpl)

func NewRelayConn(clientConn, remoteConn net.Conn, opts ...RelayConnOption) RelayConn {
	rci := &relayConnImpl{
		clientConn: clientConn,
		remoteConn: remoteConn,
		Stats:      &Stats{},
	}
	for _, opt := range opts {
		opt(rci)
	}
	if rci.l == nil {
		rci.l = zap.S().Named(rci.RelayLabel)
	}
	return rci
}

type relayConnImpl struct {
	clientConn net.Conn
	remoteConn net.Conn

	Closed bool `json:"closed"`

	Stats     *Stats    `json:"stats"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// options set those fields
	l          *zap.SugaredLogger
	remote     *lb.Node
	RelayLabel string `json:"relay_label"`
	ConnType   string `json:"conn_type"`
	Options    *conf.Options
}

func WithRelayLabel(relayLabel string) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.RelayLabel = relayLabel
	}
}

func WithConnType(connType string) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.ConnType = connType
	}
}

func WithRemote(remote *lb.Node) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.remote = remote
		rci.Stats.HandShakeLatency = remote.HandShakeDuration
	}
}

func WithLogger(l *zap.SugaredLogger) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.l = l
	}
}

func WithRelayOptions(opts *conf.Options) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.Options = opts
	}
}

func (rc *relayConnImpl) Transport() error {
	defer func() {
		err := rc.Close()
		if err != nil {
			rc.l.Errorf("Error closing Transport connection: %s", err)
		}
	}()

	rc.l = rc.l.Named(shortHashSHA256(rc.GetFlow()))
	rc.l.Debugf("Starting transport: %s <-> %s", rc.clientConn.RemoteAddr(), rc.remoteConn.RemoteAddr())

	clientConn := newInnerConn(rc.clientConn, rc)
	clientConn.l = rc.l.Named("client")
	remoteConn := newInnerConn(rc.remoteConn, rc)
	remoteConn.l = rc.l.Named("remote")

	rc.StartTime = time.Now().Local()
	err := copyConn(clientConn, remoteConn, rc.l)
	rc.EndTime = time.Now().Local()

	if err != nil {
		// wrap error with client and remote address
		err = fmt.Errorf("(client: %s, remote: %s) %w", clientConn.RemoteAddr(), remoteConn.RemoteAddr(), err)
	}
	rc.l.Debugf("Transport ended Connection details: client=%s, remote=%s, duration=%v, stats=%s",
		clientConn.RemoteAddr(), remoteConn.RemoteAddr(), rc.EndTime.Sub(rc.StartTime), rc.Stats)
	return err
}

func (rc *relayConnImpl) Close() error {
	err1 := rc.clientConn.Close()
	err2 := rc.remoteConn.Close()
	rc.Closed = true
	return combineErrorsAndMuteIDLE(err1, err2)
}

// functions that for web ui
func (rc *relayConnImpl) GetTime() string {
	if rc.EndTime.IsZero() {
		return fmt.Sprintf("%s - N/A", rc.StartTime.Format(time.Stamp))
	}
	return fmt.Sprintf("%s - %s", rc.StartTime.Format(time.Stamp), rc.EndTime.Format(time.Stamp))
}

func (rc *relayConnImpl) GetFlow() string {
	return fmt.Sprintf("%s <-> %s", rc.clientConn.RemoteAddr(), rc.remoteConn.RemoteAddr())
}

func (rc *relayConnImpl) GetRelayLabel() string {
	return rc.RelayLabel
}

func (rc *relayConnImpl) GetStats() *Stats {
	return rc.Stats
}

func (rc *relayConnImpl) GetConnType() string {
	return rc.ConnType
}

type Stats struct {
	Up               int64
	Down             int64
	HandShakeLatency time.Duration
}

func (s *Stats) Record(up, down int64) {
	s.Up += up
	s.Down += down
}

func (s *Stats) String() string {
	return fmt.Sprintf("↑%s ↓%s ⏱%dms",
		bytes.PrettyByteSize(float64(s.Up)),
		bytes.PrettyByteSize(float64(s.Down)),
		s.HandShakeLatency.Milliseconds(),
	)
}

// note that innerConn is a wrapper around net.Conn to allow io.Copy to be used
type innerConn struct {
	net.Conn

	lastActive time.Time
	rc         *relayConnImpl
	l          *zap.SugaredLogger
}

func newInnerConn(conn net.Conn, rc *relayConnImpl) *innerConn {
	return &innerConn{Conn: conn, rc: rc, lastActive: time.Now().Local(), l: zap.S()}
}

func (c *innerConn) recordStats(n int, isRead bool) {
	if c.rc == nil {
		return
	}
	if isRead {
		labels := []string{c.rc.RelayLabel, c.rc.ConnType, metrics.METRIC_FLOW_READ, c.rc.remote.Address}
		metrics.NetWorkTransmitBytes.WithLabelValues(labels...).Add(float64(n))
		c.rc.Stats.Record(0, int64(n))
	} else {
		labels := []string{c.rc.RelayLabel, c.rc.ConnType, metrics.METRIC_FLOW_WRITE, c.rc.remote.Address}
		metrics.NetWorkTransmitBytes.WithLabelValues(labels...).Add(float64(n))
		c.rc.Stats.Record(int64(n), 0)
	}
}

func (c *innerConn) Read(p []byte) (n int, err error) {
	for {
		deadline := time.Now().Add(c.rc.Options.ReadTimeout)
		if err := c.Conn.SetReadDeadline(deadline); err != nil {
			return 0, err
		}
		n, err = c.Conn.Read(p)
		if err == nil {
			c.recordStats(n, true)
			c.lastActive = time.Now().Local()
			return n, err
		} else {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				since := time.Since(c.lastActive)
				if since > c.rc.Options.IdleTimeout {
					c.l.Debugf("Read idle, close remote: %s", c.rc.remote.Address)
					return 0, ErrIdleTimeout
				}
				continue
			}
			return 0, err
		}
	}
}

func (c *innerConn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	if err == nil {
		c.recordStats(n, false)
		now := time.Now().Local()
		c.lastActive = now
	}
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
	return hex.EncodeToString(hash)[:shortHashLength]
}

func copyConn(conn1, conn2 *innerConn, l *zap.SugaredLogger) error {
	buf1 := buffer.BufferPool.Get()
	defer buffer.BufferPool.Put(buf1)
	buf2 := buffer.BufferPool.Get()
	defer buffer.BufferPool.Put(buf2)

	errCH := make(chan error, 1)
	// copy conn1 to conn2, read from conn1 and write to conn2
	go func() {
		_, err := io.CopyBuffer(conn2, conn1, buf1)
		_ = conn2.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		if err != nil {
			conn1.l.Debugf("Error in conn1 -> conn2 direction: read from %s, write to %s, error: %v", conn1.RemoteAddr(), conn2.RemoteAddr(), err)
		}
		errCH <- err
	}()

	// reverse copy conn2 to conn1, read from conn2 and write to conn1
	_, err := io.CopyBuffer(conn1, conn2, buf2)
	if err != nil {
		l.Debugf("Error in conn2 -> conn1 direction: read from %s, write to %s, error: %v", conn2.RemoteAddr(), conn1.RemoteAddr(), err)
	}
	_ = conn1.CloseWrite()
	err2 := <-errCH
	_ = conn1.CloseRead()
	_ = conn2.CloseRead()
	return combineErrorsAndMuteIDLE(err, err2)
}

func combineErrorsAndMuteIDLE(err1, err2 error) error {
	if err1 == ErrIdleTimeout {
		err1 = nil
	}
	if err2 == ErrIdleTimeout {
		return nil
	}
	if err1 != nil && err2 != nil {
		return errors.Join(err1, err2)
	}
	if err1 != nil {
		return err1
	}
	return err2
}
