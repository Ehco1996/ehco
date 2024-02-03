package transporter

import (
	"fmt"
	"io"
	"net"

	"github.com/Ehco1996/ehco/internal/web"
	"go.uber.org/zap"
)

type RelayConn interface {
	// Transport transports data between the client and the remote server.
	// The remoteLabel is the label of the remote server.
	Transport(remoteLabel string) error

	// ToJSON returns the JSON representation of the connection.
	// ToJSON() string
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

	err := transport(rc.clientConn, rc.remoteConn, remoteLabel, rc.Stats)
	if err != nil {
		cl.Error("transport error", zap.Error(err))
	}
	cl.Debug("transport end", zap.String("stats", rc.Stats.String()))
	return err
}

func (rc *relayConnImpl) Name() string {
	return fmt.Sprintf("c1:[%s] c2:[%s]", connectionName(rc.clientConn), connectionName(rc.remoteConn))
}

type readOnlyConn struct {
	io.Reader
	remoteLabel string
	stats       *Stats
}

func (r readOnlyConn) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	// increment the metric for the read bytes
	web.NetWorkTransmitBytes.WithLabelValues(
		r.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	// record the traffic
	r.stats.Record(int64(n), 0)
	return
}

type writeOnlyConn struct {
	io.Writer
	remoteLabel string
	stats       *Stats
}

func (w writeOnlyConn) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	web.NetWorkTransmitBytes.WithLabelValues(
		w.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	w.stats.Record(0, int64(n))
	return
}

// Note that this code assumes that conn1 is the connection to the client and conn2 is the connection to the remote server.
// leave some optimization chance for future
// * use io.CopyBuffer
// * use go routine pool
func transport(conn1, conn2 net.Conn, remoteLabel string, stats *Stats) error {
	errCH := make(chan error, 1)
	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		_, err := io.Copy(
			writeOnlyConn{Writer: conn2, stats: stats, remoteLabel: remoteLabel},
			readOnlyConn{Reader: conn1, stats: stats, remoteLabel: remoteLabel},
		)
		if tcpConn, ok := conn2.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		}
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	_, err := io.Copy(
		writeOnlyConn{Writer: conn1, stats: stats, remoteLabel: remoteLabel},
		readOnlyConn{Reader: conn2, stats: stats, remoteLabel: remoteLabel},
	)
	if tcpConn, ok := conn1.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}

	err2 := <-errCH
	// due to closeWrite, the other side will get EOF, so close the read side of conn1 and conn2
	if tcpConn, ok := conn1.(*net.TCPConn); ok {
		_ = tcpConn.CloseRead()
	}
	if tcpConn, ok := conn2.(*net.TCPConn); ok {
		_ = tcpConn.CloseRead()
	}

	// handle errors, need to combine errors from both directions
	if err != nil && err2 != nil {
		err = fmt.Errorf("transport errors in both directions: %v, %v", err, err2)
	}
	if err != nil {
		return err
	}
	return err2
}
