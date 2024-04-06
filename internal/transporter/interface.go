package transporter

import (
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/lb"
)

// RelayTransporter
type RelayTransporter interface {
	dialRemote(remote *lb.Node) (net.Conn, error)
	HandleTCPConn(c net.Conn, remote *lb.Node) error
	GetRemote() *lb.Node
}

func NewRelayTransporter(cfg *conf.Config, connMgr cmgr.Cmgr) RelayTransporter {
	tcpNodeList := make([]*lb.Node, len(cfg.TCPRemotes))
	for idx, addr := range cfg.TCPRemotes {
		tcpNodeList[idx] = &lb.Node{
			Address: addr,
			Label:   fmt.Sprintf("%s-%s", cfg.Label, addr),
		}
	}
	raw := newRawClient(cfg.Label, lb.NewRoundRobin(tcpNodeList), connMgr)
	switch cfg.TransportType {
	case constant.Transport_RAW:
		return raw
	case constant.Transport_WS:
		return newWsClient(raw)
	case constant.Transport_WSS:
		return newWSSClient(raw)
	case constant.Transport_MWSS:
		return newMWSSClient(raw)
	case constant.Transport_MTCP:
		return newMTCPClient(raw)
	}
	return nil
}
