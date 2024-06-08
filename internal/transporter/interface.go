package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type TCPHandShakeF func(remote *lb.Node) (net.Conn, error)

type RelayClient interface {
	TCPHandShake(remote *lb.Node) (net.Conn, error)
	RelayTCPConn(c net.Conn, handshakeF TCPHandShakeF) error
}

func newRelayClient(base *baseTransporter) (RelayClient, error) {
	switch base.cfg.TransportType {
	case constant.RelayTypeRaw:
		return newRawClient(base)
	case constant.RelayTypeWS:
		return newWsClient(base)
	case constant.RelayTypeMWS:
		return newMwsClient(base)
	case constant.RelayTypeWSS:
		return newWssClient(base)
	case constant.RelayTypeMWSS:
		return newMwssClient(base)
	case constant.RelayTypeMTCP:
		return newMtcpClient(base)
	default:
		panic("unsupported transport type" + base.cfg.TransportType)
	}
}

type RelayServer interface {
	ListenAndServe() error
	Close() error
}

func NewRelayServer(cfg *conf.Config, cmgr cmgr.Cmgr) (RelayServer, error) {
	base := NewBaseTransporter(cfg, cmgr)
	switch cfg.ListenType {
	case constant.RelayTypeRaw:
		return newRawServer(base)
	case constant.RelayTypeWS:
		return newWsServer(base)
	case constant.RelayTypeMWS:
		return newMwsServer(base)
	case constant.RelayTypeWSS:
		return newWssServer(base)
	case constant.RelayTypeMWSS:
		return newMwssServer(base)
	case constant.RelayTypeMTCP:
		return newMtcpServer(base)
	default:
		panic("unsupported transport type" + cfg.ListenType)
	}
}
