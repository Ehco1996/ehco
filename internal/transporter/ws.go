package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
)

type Ws struct {
	raw Raw
}

func (s *Ws) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Ws) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Ws) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()
	web.CurTCPNum.Inc()
	defer web.CurTCPNum.Dec()

	node := s.raw.TCPNodes.PickMin()
	defer s.raw.TCPNodes.DeferPick(node)

	wsc, _, _, err := ws.Dial(context.TODO(), node.Remote+"/ws/")
	if err != nil {
		s.raw.TCPNodes.OnError(node)
		return err
	}
	defer wsc.Close()
	logger.Infof("[ws] HandleTCPConn from %s to %s", c.LocalAddr().String(), node.Remote)
	return transport(c, wsc)
}
