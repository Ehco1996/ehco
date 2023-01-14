package transporter

import (
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

func (raw *Raw) HandleUDPConn(c net.Conn, remote *lb.Node) error {
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_UDP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_UDP).Dec()
	remoteUdp, _ := net.ResolveUDPAddr("udp", remote.Address)
	rc, err := net.DialUDP("udp", nil, remoteUdp)
	if err != nil {
		return err
	}
	return transport(c, rc, remote.Label)
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
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Dec()

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
