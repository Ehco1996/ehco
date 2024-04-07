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

func NewRelayClient(tpType string, base *baseTransporter) (RelayClient, error) {
	switch tpType {
	case constant.Transport_RAW:
		return newRawClient(base)
	case constant.Transport_WS:
		return newWsClient(base)
	case constant.Transport_WSS:
		return newWssClient(base)
	// case constant.Transport_MWSS:
	// 	return newMWSSClient(raw)
	// case constant.Transport_MTCP:
	// 	return newMTCPClient(raw)
	default:
		panic("unsupported transport type")
	}
}

type RelayServer interface {
	ListenAndServe() error
	Close() error
}

func NewRelayServer(tpType string, base *baseTransporter) (RelayServer, error) {
	switch tpType {
	case constant.Transport_RAW:
		return newRawServer(base)
	case constant.Transport_WS:
		return newWsServer(base)
	case constant.Transport_WSS:
		return newWssServer(base)
	// case constant.Transport_MWSS:
	// 	return newMWSSServer(raw)
	// case constant.Transport_MTCP:
	// 	return newMTCPServer(raw)
	default:
		panic("unsupported transport type")
	}
}
