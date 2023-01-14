package transporter

import (
	"net"
	"time"

	"github.com/traefik/traefik/v2/pkg/udp"
)

type WrapperUdpConn struct {
	udp.Conn
	local  net.Addr
	remote net.Addr
}

var _ net.Conn = (*WrapperUdpConn)(nil)

func NewWrapperUdpConn(underC *udp.Conn, remoteAddr, localAddr net.Addr) *WrapperUdpConn {
	return &WrapperUdpConn{
		Conn:   *underC,
		local:  localAddr,
		remote: remoteAddr,
	}
}

func (wp *WrapperUdpConn) LocalAddr() net.Addr {
	return wp.local
}

func (wp *WrapperUdpConn) RemoteAddr() net.Addr {
	return wp.remote
}

func (wp *WrapperUdpConn) Close() error {
	return wp.Conn.Close()
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
