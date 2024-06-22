package transporter

import (
	"context"
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type TCPHandShakeF func(remote *lb.Node) (net.Conn, error)

type RelayClient interface {
	HealthCheck(ctx context.Context, remote *lb.Node) error
	TCPHandShake(remote *lb.Node) (net.Conn, error)
}

func newRelayClient(cfg *conf.Config) (RelayClient, error) {
	switch cfg.TransportType {
	case constant.RelayTypeRaw:
		return newRawClient(cfg)
	case constant.RelayTypeMTCP:
		return newMtcpClient(cfg)
	case constant.RelayTypeWS:
		return newWsClient(cfg)
	case constant.RelayTypeMWS:
		return newMwsClient(cfg)
	case constant.RelayTypeWSS:
		return newWssClient(cfg)
	case constant.RelayTypeMWSS:
		return newMwssClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.TransportType)
	}
}

type RelayServer interface {
	ListenAndServe() error
	Close() error
	HealthCheck(ctx context.Context) error
}

func NewRelayServer(cfg *conf.Config, cmgr cmgr.Cmgr) (RelayServer, error) {
	base, err := NewBaseTransporter(cfg, cmgr)
	if err != nil {
		return nil, err
	}
	switch cfg.ListenType {
	case constant.RelayTypeRaw:
		return newRawServer(base)
	case constant.RelayTypeMTCP:
		return newMtcpServer(base)
	case constant.RelayTypeWS:
		return newWsServer(base)
	case constant.RelayTypeMWS:
		return newMwsServer(base)
	case constant.RelayTypeWSS:
		return newWssServer(base)
	case constant.RelayTypeMWSS:
		return newMwssServer(base)
	default:
		panic("unsupported transport type" + cfg.ListenType)
	}
}
