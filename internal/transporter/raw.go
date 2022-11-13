package transporter

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/web"
	"go.uber.org/zap"
)

type Raw struct {
	udpmu          sync.Mutex
	TCPRemotes     lb.RoundRobin
	UDPRemotes     lb.RoundRobin
	UDPBufferChMap map[string]*BufferCh

	L *zap.SugaredLogger
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
		raw.L.Error(err)
		return
	}
	defer func() {
		rc.Close()
		raw.udpmu.Lock()
		delete(raw.UDPBufferChMap, uaddr.String())
		raw.udpmu.Unlock()
	}()

	raw.L.Infof("HandleUDPConn from %s to %s", local.LocalAddr().String(), remote.Label)

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
				raw.L.Error(err)
				break
			}
			if _, err := local.WriteToUDP(buf[0:i], uaddr); err != nil {
				raw.L.Error(err)
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
				raw.L.Error(err)
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

func (raw *Raw) dialRemote(remote *lb.Node) (net.Conn, error) {
	d := net.Dialer{Timeout: constant.DialTimeOut}
	rc, err := d.Dial("tcp", remote.Address)
	if err != nil {
		raw.L.Errorf("dial error: %s", err)
		return nil, err
	}
	return rc, nil
}

func (raw *Raw) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()

	defer c.Close()
	t1 := time.Now()
	rc, err := raw.dialRemote(remote)
	web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	if err != nil {
		return err
	}
	raw.L.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	defer rc.Close()
	return transport(rc, c, remote.Label)
}
