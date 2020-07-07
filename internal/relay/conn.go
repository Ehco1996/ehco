package relay

import (
	"net"
	"time"

	"github.com/xtaci/smux"
)

// Deadliner is a wrapper around net.Conn that sets read/write deadlines before
// every Read() or Write() call.
type Deadliner struct {
	net.Conn
	t time.Duration
}

func (d Deadliner) Write(p []byte) (int, error) {
	if err := d.Conn.SetWriteDeadline(time.Now().Add(d.t)); err != nil {
		return 0, err
	}
	return d.Conn.Write(p)
}

func (d Deadliner) Read(p []byte) (int, error) {
	if err := d.Conn.SetReadDeadline(time.Now().Add(d.t)); err != nil {
		return 0, err
	}
	return d.Conn.Read(p)
}

func NewDeadLinerConn(c net.Conn, t time.Duration) *Deadliner {
	return &Deadliner{c, t}
}

// mux steam whit deadline
type muxDeadlineStreamConn struct {
	net.Conn
	stream *smux.Stream
	t      time.Duration
}

func newMuxDeadlineStreamConn(
	conn net.Conn, stream *smux.Stream, t time.Duration) *muxDeadlineStreamConn {
	return &muxDeadlineStreamConn{Conn: conn, stream: stream, t: t}
}

func (c *muxDeadlineStreamConn) Read(b []byte) (n int, err error) {
	if err := c.stream.SetReadDeadline(time.Now().Add(c.t)); err != nil {
		return 0, err
	}
	return c.stream.Read(b)
}

func (c *muxDeadlineStreamConn) Write(b []byte) (n int, err error) {
	if err := c.stream.SetWriteDeadline(time.Now().Add(c.t)); err != nil {
		return 0, err
	}
	return c.stream.Write(b)
}

func (c *muxDeadlineStreamConn) Close() error {
	return c.stream.Close()
}

type muxSession struct {
	conn         net.Conn
	session      *smux.Session
	maxStreamCnt int
	t            time.Duration
}

func (session *muxSession) GetConn() (net.Conn, error) {
	stream, err := session.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return newMuxDeadlineStreamConn(session.conn, stream, session.t), nil
}

func (session *muxSession) Close() error {
	if session.session == nil {
		return nil
	}
	return session.session.Close()
}

func (session *muxSession) IsClosed() bool {
	if session.session == nil {
		return true
	}
	return session.session.IsClosed()
}

func (session *muxSession) NumStreams() int {
	if session.session != nil {
		return session.session.NumStreams()
	}
	return 0
}
