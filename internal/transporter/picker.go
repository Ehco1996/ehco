package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/pkg/limiter"
)

// RelayTransporter
type RelayTransporter interface {

	// UDP相关
	GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh
	HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn)

	// TCP相关
	HandleTCPConn(c *net.TCPConn, remote *lb.Node) error
	GetRemote() *lb.Node

	// limit
	LimitByIp(c *net.TCPConn) error
}

func PickTransporter(transType string, tcpRemotes, udpRemotes lb.RoundRobin) RelayTransporter {
	raw := Raw{
		TCPRemotes:     tcpRemotes,
		UDPRemotes:     udpRemotes,
		UDPBufferChMap: make(map[string]*BufferCh),

		ipLimiter: limiter.NewIPRateLimiter(constant.TCP_RATE_LIMIT, constant.TCP_RATE_LIMIT),
	}
	switch transType {
	case constant.Transport_RAW:
		return &raw
	case constant.Transport_WS:
		return &Ws{raw: &raw}
	case constant.Transport_WSS:
		return &Wss{raw: &raw}
	case constant.Transport_MWSS:
		return &Mwss{raw: &raw, mtp: NewMWSSTransporter()}
	}
	return nil
}
