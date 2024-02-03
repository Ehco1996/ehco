// nolint: errcheck
package transporter

import (
	"context"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type Raw struct {
	relayLabel string

	// TCP
	cmgr       cmgr.Cmgr
	tCPRemotes lb.RoundRobin

	// UDP todo refactor udp relay
	udpmu          sync.Mutex
	uDPRemotes     lb.RoundRobin
	uDPBufferChMap map[string]*BufferCh

	l *zap.SugaredLogger
}

func newRaw(
	relayLabel string,
	tcpRemotes, udpRemotes lb.RoundRobin,
	cmgr cmgr.Cmgr,
) *Raw {
	r := &Raw{
		cmgr: cmgr,

		relayLabel:     relayLabel,
		tCPRemotes:     tcpRemotes,
		uDPRemotes:     udpRemotes,
		uDPBufferChMap: make(map[string]*BufferCh),

		l: zap.S().Named(relayLabel),
	}
	return r
}

func (raw *Raw) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	raw.udpmu.Lock()
	defer raw.udpmu.Unlock()

	bc, found := raw.uDPBufferChMap[uaddr.String()]
	if !found {
		bc := newudpBufferCh(uaddr)
		raw.uDPBufferChMap[uaddr.String()] = bc
		return bc
	}
	return bc
}

func (raw *Raw) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	remote := raw.uDPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_UDP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_UDP).Dec()

	bc := raw.GetOrCreateBufferCh(uaddr)
	remoteUdp, _ := net.ResolveUDPAddr("udp", remote.Address)
	rc, err := net.DialUDP("udp", nil, remoteUdp)
	if err != nil {
		raw.l.Error(err)
		return
	}
	defer func() {
		rc.Close()
		raw.udpmu.Lock()
		delete(raw.uDPBufferChMap, uaddr.String())
		raw.udpmu.Unlock()
	}()

	raw.l.Infof("HandleUDPConn from %s to %s", local.LocalAddr().String(), remote.Label)

	buf := BufferPool.Get()
	defer BufferPool.Put(buf)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer wg.Done()
		defer cancel()
		for {
			_ = rc.SetDeadline(time.Now().Add(constant.IdleTimeOut))
			i, err := rc.Read(buf)
			if err != nil {
				raw.l.Error(err)
				break
			}
			web.NetWorkTransmitBytes.WithLabelValues(
				remote.Label, web.METRIC_CONN_TYPE_UDP, web.METRIC_CONN_FLOW_READ,
			).Add(float64(i))

			if _, err := local.WriteToUDP(buf[0:i], uaddr); err != nil {
				raw.l.Error(err)
				break
			}
			web.NetWorkTransmitBytes.WithLabelValues(
				remote.Label, web.METRIC_CONN_TYPE_UDP, web.METRIC_CONN_FLOW_WRITE,
			).Add(float64(i))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		case b := <-bc.Ch:
			// read from local udp listener ch
			web.NetWorkTransmitBytes.WithLabelValues(
				remote.Label, web.METRIC_CONN_TYPE_UDP, web.METRIC_CONN_FLOW_READ,
			).Add(float64(len(b)))

			_ = rc.SetDeadline(time.Now().Add(constant.IdleTimeOut))
			if _, err := rc.Write(b); err != nil {
				raw.l.Error(err)
				return
			}
			web.NetWorkTransmitBytes.WithLabelValues(
				remote.Label, web.METRIC_CONN_TYPE_UDP, web.METRIC_CONN_FLOW_WRITE,
			).Add(float64(len(b)))
		}
	}()
	wg.Wait()
}

func (raw *Raw) GetRemote() *lb.Node {
	return raw.tCPRemotes.Next()
}

func (raw *Raw) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	d := net.Dialer{Timeout: constant.DialTimeOut}
	rc, err := d.Dial("tcp", remote.Address)
	if err != nil {
		return nil, err
	}
	web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	return rc, nil
}

func (raw *Raw) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	// todo refactor metrics to server
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Dec()

	defer c.Close()
	rc, err := raw.dialRemote(remote)
	if err != nil {
		return err
	}
	defer rc.Close()

	raw.l.Infof("HandleTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(raw.relayLabel, c, rc)
	raw.cmgr.AddConnection(relayConn)
	return relayConn.Transport(remote.Label)
}
