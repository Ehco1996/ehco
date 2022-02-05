package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/tls"
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

func (s *Wss) HandleTCPConn(c *net.TCPConn, remote *lb.Node) error {
	defer c.Close()

	d := ws.Dialer{TLSConfig: tls.DefaultTLSConfig}
	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/wss/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	logger.Infof("[wss] HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	return transport(c, wsc, remote.Label)
}

func (s *Wss) GetRemote() *lb.Node {
	return s.raw.TCPRemotes.Next()
}
