package transporter

import (
	"context"
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/relay/conf"
)

// TODO opt this interface
type RelayClient interface {
	HandShake(ctx context.Context, remote *lb.Node, isTCP bool) (net.Conn, error)
}

func newRelayClient(cfg *conf.Config) (RelayClient, error) {
	switch cfg.TransportType {
	case constant.RelayTypeRaw:
		return newRawClient(cfg)
	case constant.RelayTypeWS:
		return newWsClient(cfg)
	case constant.RelayTypeWSS:
		return newWssClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.TransportType)
	}
}

type RelayServer interface {
	ListenAndServe(ctx context.Context) error
	Close() error

	RelayTCPConn(ctx context.Context, c net.Conn, remote *lb.Node) error
	RelayUDPConn(ctx context.Context, c net.Conn, remote *lb.Node) error
	HealthCheck(ctx context.Context) (int64, error) // latency in ms
}

func NewRelayServer(cfg *conf.Config, cmgr cmgr.Cmgr) (RelayServer, error) {
	base, err := newBaseRelayServer(cfg, cmgr)
	if err != nil {
		return nil, err
	}
	switch cfg.ListenType {
	case constant.RelayTypeRaw:
		return newRawServer(base)
	case constant.RelayTypeWS:
		return newWsServer(base)
	case constant.RelayTypeWSS:
		return newWssServer(base)
	default:
		panic("unsupported transport type" + cfg.ListenType)
	}
}
