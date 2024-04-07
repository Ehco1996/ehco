package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type TCPHandShakeF func(remote *lb.Node) (net.Conn, error)

type RelayClient interface {
	TCPHandShake(remote *lb.Node) (net.Conn, error)
	RelayTCPConn(c net.Conn, handshakeF TCPHandShakeF) error
}

func NewRelayClient(relayType string, base *baseTransporter) (RelayClient, error) {
	switch relayType {
	case constant.RelayTypeRaw:
		return newRawClient(base)
	case constant.RelayTypeWS:
		return newWsClient(base)
	case constant.RelayTypeWSS:
		return newWssClient(base)
	case constant.RelayTypeMWSS:
		return newMwssClient(base)
	case constant.RelayTypeMTCP:
		return newMtcpClient(base)
	default:
		panic("unsupported transport type")
	}
}

type RelayServer interface {
	ListenAndServe() error
	Close() error
}

func NewRelayServer(relayType string, base *baseTransporter) (RelayServer, error) {
	switch relayType {
	case constant.RelayTypeRaw:
		return newRawServer(base)
	case constant.RelayTypeWS:
		return newWsServer(base)
	case constant.RelayTypeWSS:
		return newWssServer(base)
	case constant.RelayTypeMWSS:
		return newMwssServer(base)
	case constant.RelayTypeMTCP:
		return newMtcpServer(base)
	default:
		panic("unsupported transport type")
	}
}
