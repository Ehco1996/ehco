package transporter

import (
	"net"
	"net/http"
	"sync"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/gobwas/ws"
)

type Raw struct {
	TCPNodes       *lb.LBNodes
	UDPNodes       *lb.LBNodes
	UDPBufferChMap map[string]*BufferCh
}

// NOTE not thread safe
func (raw *Raw) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	bc, found := raw.UDPBufferChMap[uaddr.String()]
	if !found {
		bc := newudpBufferCh()
		raw.UDPBufferChMap[uaddr.String()] = bc
		return bc
	}
	return bc
}

func (raw *Raw) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {

	bc := raw.GetOrCreateBufferCh(uaddr)
	node := raw.UDPNodes.PickMin()
	defer raw.UDPNodes.DeferPick(node)

	rc, err := net.Dial("udp", node.Remote)
	if err != nil {
		logger.Logger.Info(err)
		raw.UDPNodes.OnError(node)
		return
	}

	defer func() {
		rc.Close()
		close(bc.Ch)
		delete(raw.UDPBufferChMap, uaddr.String())
	}()

	logger.Logger.Infof("[raw] HandleUDPConn from %s to %s", local.LocalAddr().String(), node.Remote)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		buf := outboundBufferPool.Get().([]byte)
		for {
			i, err := rc.Read(buf)
			if err != nil {
				logger.Logger.Info(err)
				break
			}
			if _, err := local.WriteToUDP(buf[0:i], uaddr); err != nil {
				logger.Logger.Info(err)
				break
			}
		}
		outboundBufferPool.Put(buf)
		wg.Done()
	}()

	for b := range bc.Ch {
		if _, err := rc.Write(b); err != nil {
			logger.Logger.Info(err)
			break
		}
	}
	wg.Wait()
}

func (raw *Raw) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()

	node := raw.TCPNodes.PickMin()
	defer raw.TCPNodes.DeferPick(node)

	rc, err := net.Dial("tcp", node.Remote)
	if err != nil {
		raw.TCPNodes.OnError(node)
		return err
	}
	logger.Logger.Infof("[raw] HandleTCPConn from %s to %s", c.LocalAddr().String(), node.Remote)

	defer rc.Close()
	return transport(c, rc)
}

func (raw *Raw) HandleWsRequset(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()

	node := raw.TCPNodes.PickMin()
	defer raw.TCPNodes.DeferPick(node)

	rc, err := net.Dial("tcp", node.Remote)
	if err != nil {
		logger.Logger.Infof("dial error: %s", err)
		raw.TCPNodes.OnError(node)
		return
	}
	defer rc.Close()

	logger.Logger.Infof("[tun] HandleWsRequset from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	if err := transport(rc, wsc); err != nil {
		logger.Logger.Infof("[tun] HandleWsRequset err: %s", err.Error())
	}
}

func (raw *Raw) HandleWssRequset(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()

	node := raw.TCPNodes.PickMin()
	defer raw.TCPNodes.DeferPick(node)

	rc, err := net.Dial("tcp", node.Remote)
	if err != nil {
		logger.Logger.Infof("dial error: %s", err)
		raw.TCPNodes.OnError(node)
		return
	}
	defer rc.Close()

	logger.Logger.Infof("[tun] HandleWssRequset from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	if err := transport(rc, wsc); err != nil {
		logger.Logger.Infof("[tun] HandleWssRequset err: %s", err.Error())
	}
}
