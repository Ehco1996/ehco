package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
)

type Wss struct {
	raw *Raw
}

func (s *Wss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Wss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Wss) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()
	remote := s.raw.TCPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()

	d := ws.Dialer{TLSConfig: tls.DefaultTLSConfig}
	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/wss/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	logger.Infof("[wss] HandleTCPConn from %s to %s", c.LocalAddr().String(), remote.Label)
	return transport(c, wsc, remote.Label)
}
