package conn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/buffer"
	"github.com/Ehco1996/ehco/pkg/bytes"
	"go.uber.org/zap"
)

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
	l                 *zap.SugaredLogger
	remote            *lb.Node
	HandshakeDuration time.Duration
	RelayLabel        string `json:"relay_label"`
	ConnType          string `json:"conn_type"`
}

func WithRelayLabel(relayLabel string) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.RelayLabel = relayLabel
	}
}

func WithHandshakeDuration(duration time.Duration) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.HandshakeDuration = duration
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
	}
}

func WithLogger(l *zap.SugaredLogger) RelayConnOption {
	return func(rci *relayConnImpl) {
		rci.l = l
	}
}

func (rc *relayConnImpl) Transport() error {
	defer rc.Close() // nolint: errcheck
	cl := rc.l.Named(shortHashSHA256(rc.GetFlow()))
	cl.Debugf("transport start, stats: %s", rc.Stats.String())
	c1 := newInnerConn(rc.clientConn, rc)
	c2 := newInnerConn(rc.remoteConn, rc)
	rc.StartTime = time.Now().Local()
	err := copyConn(c1, c2)
	if err != nil {
		cl.Errorf("transport error: %s", err.Error())
	}
	cl.Debugf("transport end, stats: %s", rc.Stats.String())
	rc.EndTime = time.Now().Local()
	return err
}

func (rc *relayConnImpl) Close() error {
	err1 := rc.clientConn.Close()
	err2 := rc.remoteConn.Close()
	rc.Closed = true
	return combineErrorsAndMuteEOF(err1, err2)
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

func combineErrorsAndMuteEOF(err1, err2 error) error {
	if err1 == io.EOF {
		err1 = nil
	}
	if err2 == io.EOF {
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
	return fmt.Sprintf("up: %s, down: %s, handshake latency: %s",
		bytes.PrettyByteSize(float64(s.Up)),
		bytes.PrettyByteSize(float64(s.Down)),
		s.HandShakeLatency.String(),
	)
}

// note that innerConn is a wrapper around net.Conn to allow io.Copy to be used
type innerConn struct {
	net.Conn
	lastActive time.Time

	rc *relayConnImpl
}

func newInnerConn(conn net.Conn, rc *relayConnImpl) *innerConn {
	return &innerConn{Conn: conn, rc: rc, lastActive: time.Now()}
}

func (c *innerConn) recordStats(n int, isRead bool) {
	if c.rc == nil {
		return
	}
	if isRead {
		metrics.NetWorkTransmitBytes.WithLabelValues(
			c.rc.remote.Label, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_READ,
		).Add(float64(n))
		c.rc.Stats.Record(0, int64(n))
	} else {
		metrics.NetWorkTransmitBytes.WithLabelValues(
			c.rc.remote.Label, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_WRITE,
		).Add(float64(n))
		c.rc.Stats.Record(int64(n), 0)
	}
}

func (c *innerConn) Read(p []byte) (n int, err error) {
	for {
		deadline := time.Now().Add(constant.ReadTimeOut)
		if err := c.Conn.SetReadDeadline(deadline); err != nil {
			return 0, err
		}
		n, err = c.Conn.Read(p)
		if err == nil {
			c.recordStats(n, true)
			c.lastActive = time.Now()
			return
		} else {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(c.lastActive) > constant.IdleTimeOut {
					c.rc.l.Debugf("read idle,close remote: %s", c.rc.remote.Label)
					return 0, io.EOF
				}
				continue
			}
			return n, err
		}
	}
}

func (c *innerConn) Write(p []byte) (n int, err error) {
	if time.Since(c.lastActive) > constant.IdleTimeOut {
		c.rc.l.Debugf("write idle,close remote: %s", c.rc.remote.Label)
		return 0, io.EOF
	}
	n, err = c.Conn.Write(p)
	c.recordStats(n, false) // false for write operation
	if err != nil {
		c.lastActive = time.Now()
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
	return hex.EncodeToString(hash)[:7]
}

func copyConn(conn1, conn2 *innerConn) error {
	buf := buffer.BufferPool.Get()
	defer buffer.BufferPool.Put(buf)
	errCH := make(chan error, 1)
	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		_, err := io.CopyBuffer(conn2, conn1, buf)
		_ = conn2.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	buf2 := buffer.BufferPool.Get()
	defer buffer.BufferPool.Put(buf2)
	_, err := io.CopyBuffer(conn1, conn2, buf2)
	_ = conn1.CloseWrite()

	err2 := <-errCH
	_ = conn1.CloseRead()
	_ = conn2.CloseRead()
	return combineErrorsAndMuteEOF(err, err2)
}
