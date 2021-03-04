package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/web"
)

type Mwss struct {
	raw Raw
	mtp *mwssTransporter
}

func (s *Mwss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Mwss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Mwss) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()
	web.CurTCPNum.Inc()
	defer web.CurTCPNum.Dec()

	node := s.raw.TCPNodes.PickMin()
	defer s.raw.TCPNodes.DeferPick(node)

	wsc, err := s.mtp.Dial(node.Remote + "/mwss/")
	if err != nil {
		s.raw.TCPNodes.OnError(node)
		return err
	}
	defer wsc.Close()
	logger.Infof("[mwss] HandleTCPConn from:%s to:%s", c.RemoteAddr(), wsc.RemoteAddr())
	return transport(wsc, c)
}
