package relay

import (
	"net"
	"time"
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
