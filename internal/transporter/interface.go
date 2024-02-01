package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/lb"
)

// RelayTransporter
type RelayTransporter interface {
	// UDP相关
	GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh
	HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn)

	// TCP相关
	dialRemote(remote *lb.Node) (net.Conn, error)
	HandleTCPConn(c net.Conn, remote *lb.Node) error
	GetRemote() *lb.Node
}

func NewRelayTransporter(transType string, tcpRemotes, udpRemotes lb.RoundRobin) RelayTransporter {
	raw := newRawTransporter(transType, tcpRemotes, udpRemotes)
	switch transType {
	case constant.Transport_RAW:
		return raw
	case constant.Transport_WS:
		return &Ws{Raw: raw}
	case constant.Transport_WSS:
		return &Wss{Raw: raw}
	case constant.Transport_MWSS:
		logger := raw.l.Named("MWSSClient")
		mWSSClient := NewMWSSClient(logger)
		mwss := &Mwss{mtp: NewSmuxTransporter(logger, mWSSClient.InitNewSession)}
		mwss.Raw = raw
		return mwss
	case constant.Transport_MTCP:
		logger := raw.l.Named("MTCPClient")
		mTCPClient := NewMTCPClient(logger)
		mtcp := &MTCP{mtp: NewSmuxTransporter(logger, mTCPClient.InitNewSession)}
		mtcp.Raw = raw
		return mtcp
	}
	return nil
}
