package transporter

import (
	"net"

	"github.com/xtaci/smux"
)

type muxConn struct {
	net.Conn
	stream *smux.Stream
}

func newMuxConn(conn net.Conn, stream *smux.Stream) *muxConn {
	return &muxConn{Conn: conn, stream: stream}
}

func (c *muxConn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *muxConn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *muxConn) Close() error {
	return c.stream.Close()
}

type muxSession struct {
	conn         net.Conn
	session      *smux.Session
	maxStreamCnt int
}

func (session *muxSession) GetConn() (net.Conn, error) {
	stream, err := session.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return newMuxConn(session.conn, stream), nil
}

func (session *muxSession) Close() error {
	if session.session == nil {
		return nil
	}
	session.conn.Close()
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
