package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/lb"
	"go.uber.org/zap"
)

// RelayTransporter
type RelayTransporter interface {
	// client side func
	GetRemote() *lb.Node
	TCPHandShake(remote *lb.Node) (net.Conn, error)
	RelayTCPConn(c net.Conn) error

	// server side func
	ListenAndServe() error
	Close() error
}

func NewRelayTransporter(cfg *conf.Config, connMgr cmgr.Cmgr) (RelayTransporter, error) {
	base := newBaseTransporter(cfg, connMgr)

	switch cfg.TransportType {
	case constant.Transport_RAW:
		return newRawClient(base)
	// case constant.Transport_WS:
	// return newWsClient(raw)
	// case constant.Transport_WSS:
	// 	return newWSSClient(raw)
	// case constant.Transport_MWSS:
	// 	return newMWSSClient(raw)
	// case constant.Transport_MTCP:
	// 	return newMTCPClient(raw)
	default:
		panic("unsupported transport type")
	}
}

type baseTransporter struct {
	cmgr       cmgr.Cmgr
	cfg        *conf.Config
	tCPRemotes lb.RoundRobin
	l          *zap.SugaredLogger
}

func newBaseTransporter(cfg *conf.Config, cmgr cmgr.Cmgr) *baseTransporter {
	return &baseTransporter{
		cfg:        cfg,
		cmgr:       cmgr,
		tCPRemotes: cfg.ToTCPRemotes(),
		l:          zap.S().Named(cfg.GetLoggerName()),
	}
}
