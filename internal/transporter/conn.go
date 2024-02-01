package transporter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"

	"github.com/Ehco1996/ehco/internal/web"
	"go.uber.org/zap"
)

type readOnlyConn struct {
	io.Reader
	remoteAddr string
	cs         ConnStats
}

func (r readOnlyConn) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	// increment the metric for the read bytes
	web.NetWorkTransmitBytes.WithLabelValues(
		r.remoteAddr, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	// record the traffic
	r.cs.RecordTraffic(int64(n), 0)
	return
}

type writeOnlyConn struct {
	io.Writer
	remoteAddr string
	cs         ConnStats
}

func (w writeOnlyConn) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	web.NetWorkTransmitBytes.WithLabelValues(
		w.remoteAddr, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	w.cs.RecordTraffic(0, int64(n))
	return
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

// Note that this code assumes that conn1 is the connection to the client and conn2 is the connection to the remote server.
// leave some optimization chance for future
// * use io.CopyBuffer
// * use go routine pool
func transport(conn1, conn2 net.Conn, remoteAddr string, cs ConnStats) error {
	name := fmt.Sprintf("c1:[%s] c2:[%s]", connectionName(conn1), connectionName(conn2))
	l := zap.S().Named(shortHashSHA256(name))
	l.Debugf("transport for:%s start", name)
	defer l.Debugf("transport for:%s end", name)
	errCH := make(chan error, 1)

	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		l.Debug("copy conn1 to conn2 start")
		_, err := io.Copy(
			writeOnlyConn{Writer: conn2, cs: cs, remoteAddr: remoteAddr},
			readOnlyConn{Reader: conn1, cs: cs, remoteAddr: remoteAddr},
		)
		l.Debug("copy conn1 to conn2 end", err)
		if tcpConn, ok := conn2.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		}
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	l.Debug("copy conn2 to conn1 start")
	_, err := io.Copy(
		writeOnlyConn{Writer: conn1, cs: cs, remoteAddr: remoteAddr},
		readOnlyConn{Reader: conn2, cs: cs, remoteAddr: remoteAddr},
	)
	l.Debug("copy conn2 to conn1 end", err)
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
		err := fmt.Errorf("errors in both directions: %v, %v", err, err2)
		l.Error(err)
	}
	if err != nil {
		return err
	}
	return err2
}
