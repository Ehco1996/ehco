package transporter

import (
	"net"
	"time"

	"github.com/traefik/traefik/v2/pkg/udp"
	"go.uber.org/atomic"
)

type WrapperUdpConn struct {
	udp.Conn
	local  net.Addr
	remote net.Addr
	closed *atomic.Bool
}

var _ net.Conn = (*WrapperUdpConn)(nil)

func NewWrapperUdpConn(underC *udp.Conn, remoteAddr, localAddr net.Addr) *WrapperUdpConn {
	return &WrapperUdpConn{
		Conn:   *underC,
		local:  localAddr,
		remote: remoteAddr,
		closed: atomic.NewBool(false),
	}
}

func (wp *WrapperUdpConn) LocalAddr() net.Addr {
	return wp.local
}

func (wp *WrapperUdpConn) RemoteAddr() net.Addr {
	return wp.remote
}

func (wp *WrapperUdpConn) Close() error {
	println("called closed", wp.local.String())
	// if wp.closed.CAS(false, true) {
	// return wp.Conn.Close()
	// }
	// wp.Conn.Close()
	return nil
}

func (wp *WrapperUdpConn) SetDeadline(t time.Time) error {
	return nil
}

func (wp *WrapperUdpConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (wp *WrapperUdpConn) SetWriteDeadline(t time.Time) error {
	return nil
}
