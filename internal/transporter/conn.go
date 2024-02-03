package transporter

import (
	"fmt"
	"io"
	"net"
)

// readOnlyConn is a net.Conn that only implements the Read method. This is useful for the CopyConn function.
// because we want to read from one connection and write to another, but we don't want to accidentally call the Write method.
type readOnlyConn struct {
	io.Reader
}

func (r readOnlyConn) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	return
}

type writeOnlyConn struct {
	io.Writer
}

func (w writeOnlyConn) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	return
}

// Note that this code assumes that conn1 is the connection to the client and conn2 is the connection to the remote server.
// leave some optimization chance for future
// * use io.CopyBuffer
// * use go routine pool
func CopyConn(conn1, conn2 net.Conn) error {
	errCH := make(chan error, 1)
	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		_, err := io.Copy(writeOnlyConn{Writer: conn2}, readOnlyConn{Reader: conn1})
		if tcpConn, ok := conn2.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		}
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	_, err := io.Copy(writeOnlyConn{Writer: conn1}, readOnlyConn{Reader: conn2})
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
