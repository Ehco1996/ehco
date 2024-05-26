package transporter

import (
	"context"
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/sagernet/sing-box/common/sniff"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"

	"github.com/Ehco1996/ehco/pkg/lb"
	"go.uber.org/zap"
)

type baseTransporter struct {
	cmgr       cmgr.Cmgr
	cfg        *conf.Config
	tCPRemotes lb.RoundRobin
	l          *zap.SugaredLogger
}

func NewBaseTransporter(cfg *conf.Config, cmgr cmgr.Cmgr) *baseTransporter {
	return &baseTransporter{
		cfg:        cfg,
		cmgr:       cmgr,
		tCPRemotes: cfg.ToTCPRemotes(),
		l:          zap.S().Named(cfg.GetLoggerName()),
	}
}

func (b *baseTransporter) GetTCPListenAddr() (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp", b.cfg.Listen)
}

func (b *baseTransporter) GetRemote() *lb.Node {
	return b.tCPRemotes.Next()
}

func (b *baseTransporter) RelayTCPConn(c net.Conn, handshakeF TCPHandShakeF) error {
	remote := b.GetRemote()
	metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Inc()
	defer metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Dec()

	// check limit
	if b.cfg.MaxConnection > 0 && b.cmgr.CountConnection(cmgr.ConnectionTypeActive) >= b.cfg.MaxConnection {
		c.Close()
		return fmt.Errorf("relay:%s active connection count exceed limit %d", b.cfg.Label, b.cfg.MaxConnection)
	}

	// sniff protocol
	if len(b.cfg.BlockedProtocols) > 0 {
		buffer := buf.NewPacket()
		ctx := context.TODO()
		sniffMetadata, err := sniff.PeekStream(
			ctx, c, buffer, constant.SniffTimeOut,
			sniff.TLSClientHello, sniff.HTTPHost)
		if err != nil {
			// this mean no protocol sniffed
			b.l.Debug("sniff error: %s", err)
		}
		if sniffMetadata != nil {
			b.l.Infof("sniffed protocol: %s", sniffMetadata.Protocol)
			for _, p := range b.cfg.BlockedProtocols {
				if sniffMetadata.Protocol == p {
					c.Close()
					return fmt.Errorf("relay:%s want to  relay blocked protocol:%s", b.cfg.Label, sniffMetadata.Protocol)
				}
			}
		}
		if !buffer.IsEmpty() {
			c = bufio.NewCachedConn(c, buffer)
		} else {
			buffer.Release()
		}
	}

	clonedRemote := remote.Clone()
	rc, err := handshakeF(clonedRemote)
	if err != nil {
		return err
	}

	b.l.Infof("RelayTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(
		b.cfg.Label, c, rc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	b.cmgr.AddConnection(relayConn)
	defer b.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}
