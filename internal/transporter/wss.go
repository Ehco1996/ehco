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
	raw Raw
}

func (s *Wss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Wss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Wss) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()
	web.CurTCPNum.Inc()
	defer web.CurTCPNum.Dec()

	node := s.raw.TCPNodes.PickMin()
	defer s.raw.TCPNodes.DeferPick(node)

	d := ws.Dialer{TLSConfig: tls.DefaultTLSConfig}
	wsc, _, _, err := d.Dial(context.TODO(), node.Remote+"/wss/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	logger.Infof("[ws] HandleTCPConn from %s to %s", c.LocalAddr().String(), node.Remote)
	return transport(c, wsc)
}
