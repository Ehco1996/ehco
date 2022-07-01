package transporter

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/limiter"
	"github.com/gobwas/ws"
)

type Raw struct {
	udpmu          sync.Mutex
	TCPRemotes     lb.RoundRobin
	UDPRemotes     lb.RoundRobin
	UDPBufferChMap map[string]*BufferCh

	ipLimiter *limiter.IPRateLimiter
}

func (raw *Raw) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	raw.udpmu.Lock()
	defer raw.udpmu.Unlock()

	bc, found := raw.UDPBufferChMap[uaddr.String()]
	if !found {
		bc := newudpBufferCh(uaddr)
		raw.UDPBufferChMap[uaddr.String()] = bc
		return bc
	}
	return bc
}

func (raw *Raw) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	remote := raw.UDPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_UDP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_UDP).Dec()

	bc := raw.GetOrCreateBufferCh(uaddr)
	remoteUdp, _ := net.ResolveUDPAddr("udp", remote.Address)
	rc, err := net.DialUDP("udp", nil, remoteUdp)
	if err != nil {
		remote.BlockForSomeTime()
		logger.Info(err)
		return
	}
	defer func() {
		rc.Close()
		raw.udpmu.Lock()
		delete(raw.UDPBufferChMap, uaddr.String())
		raw.udpmu.Unlock()
	}()

	logger.Infof("[raw] HandleUDPConn from %s to %s", local.LocalAddr().String(), remote.Label)

	buf := BufferPool.Get()
	defer BufferPool.Put(buf)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer wg.Done()
		defer cancel()
		wt := 0
		for {
			_ = rc.SetDeadline(time.Now().Add(constant.IdleTimeOut))
			i, err := rc.Read(buf)
			if err != nil {
				logger.Info(err)
				break
			}
			if _, err := local.WriteToUDP(buf[0:i], uaddr); err != nil {
				logger.Info(err)
				break
			}
			wt += i
		}
		web.NetWorkTransmitBytes.WithLabelValues(remote.Label, web.METRIC_CONN_UDP).Add(float64(wt * 2))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		wt := 0
		select {
		case <-ctx.Done():
			return
		case b := <-bc.Ch:
			wt += len(b)
			web.NetWorkTransmitBytes.WithLabelValues(remote.Label, web.METRIC_CONN_UDP).Add(float64(wt * 2))
			if _, err := rc.Write(b); err != nil {
				logger.Info(err)
				return
			}
			_ = rc.SetDeadline(time.Now().Add(constant.IdleTimeOut))
		}
	}()
	wg.Wait()
}

func (raw *Raw) GetRemote() *lb.Node {
	return raw.TCPRemotes.Next()
}

func (raw *Raw) DialRemote(remote *lb.Node) (net.Conn, error) {
	d := net.Dialer{Timeout: constant.DialTimeOut}
	rc, err := d.Dial("tcp", remote.Address)
	if err != nil {
		remote.BlockForSomeTime()
		logger.Errorf("dial error: %s", err)
		return nil, err
	}
	return rc, nil
}

func (raw *Raw) HandleTCPConn(c *net.TCPConn, remote *lb.Node) error {
	defer c.Close()
	rc, err := raw.DialRemote(remote)
	if err != nil {
		return err
	}
	logger.Infof("[raw] HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	defer rc.Close()
	return transport(rc, c, remote.Label)
}

func (raw *Raw) HandleWsRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()
	remote := raw.TCPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()
	rc, err := raw.DialRemote(remote)
	if err != nil {
		return
	}
	defer rc.Close()
	logger.Infof("[tun] HandleWsRequest from:%s to:%s", wsc.RemoteAddr(), remote.Address)
	if err := transport(rc, wsc, remote.Label); err != nil {
		logger.Infof("[tun] HandleWsRequest meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err.Error())
	}
}

func (raw *Raw) HandleWssRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()
	remote := raw.TCPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()

	rc, err := raw.DialRemote(remote)
	if err != nil {
		return
	}
	defer rc.Close()
	logger.Infof("[tun] HandleWssRequest from:%s to:%s", wsc.RemoteAddr(), remote.Address)
	if err := transport(rc, wsc, remote.Label); err != nil {
		logger.Infof("[tun] HandleWssRequest meet error from:%s to:%s err:%s", wsc.LocalAddr(), remote.Label, err.Error())
	}
}

func (raw *Raw) HandleMWssRequest(wsc net.Conn) {
	defer wsc.Close()
	remote := raw.TCPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()

	rc, err := raw.DialRemote(remote)
	if err != nil {
		return
	}
	defer rc.Close()
	logger.Infof("[tun] HandleMWssRequest from:%s to:%s", wsc.RemoteAddr(), remote.Address)
	if err := transport(wsc, rc, remote.Label); err != nil {
		logger.Infof("[tun] HandleMWssRequest meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Label, err.Error())
	}
}
