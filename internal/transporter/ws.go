package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/gobwas/ws"
)

type Ws struct {
	raw *Raw
}

func (s *Ws) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Ws) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Ws) HandleTCPConn(c *net.TCPConn, remote *lb.Node) error {
	defer c.Close()

	wsc, _, _, err := ws.Dial(context.TODO(), remote.Address+"/ws/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	s.raw.L.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	return transport(c, wsc, remote.Label)
}

func (s *Ws) GetRemote() *lb.Node {
	return s.raw.GetRemote()
}
