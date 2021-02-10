package transporter

import (
	"context"
	"net"
	"net/http"

	"github.com/Ehco1996/ehco/internal/logger"
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

	node := s.raw.TCPNodes.PickMin()
	defer s.raw.TCPNodes.DeferPick(node)

	wsc, _, _, err := ws.Dial(context.TODO(), node.Remote+"/ws/")
	if err != nil {
		s.raw.TCPNodes.OnError(node)
		return err
	}
	defer wsc.Close()
	logger.Logger.Infof("[ws] HandleTCPConn from %s to %s", c.LocalAddr().String(), node.Remote)
	if err := transport(c, wsc); err != nil {
		return err
	}
	return nil
}

func (s *Ws) HandleWebRequset(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()

	node := s.raw.TCPNodes.PickMin()
	defer s.raw.TCPNodes.DeferPick(node)

	rc, err := net.Dial("tcp", node.Remote)
	if err != nil {
		logger.Logger.Infof("dial error: %s", err)
		s.raw.TCPNodes.OnError(node)
		return
	}
	defer rc.Close()

	logger.Logger.Infof("[ws tun] HandleWebRequset from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	if err := transport(rc, wsc); err != nil {
		logger.Logger.Infof("[ws tun] HandleWebRequset err: %s", err.Error())
	}
}
