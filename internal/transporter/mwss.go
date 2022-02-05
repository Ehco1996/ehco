package transporter

import (
	"net"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
)

type Mwss struct {
	raw *Raw
	mtp *mwssTransporter
}

func (s *Mwss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Mwss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Mwss) HandleTCPConn(c *net.TCPConn, remote *lb.Node) error {
	defer c.Close()
	muxwsc, err := s.mtp.Dial(remote.Address + "/mwss/")
	if err != nil {
		return err
	}
	defer muxwsc.Close()
	logger.Infof("[mwss] HandleTCPConn from:%s to:%s", c.RemoteAddr(), remote.Address)
	return transport(muxwsc, c, remote.Label)
}

func (s *Mwss) GetRemote() *lb.Node {
	return s.raw.TCPRemotes.Next()
}
