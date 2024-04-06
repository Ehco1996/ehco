// nolint: errcheck
package transporter

import (
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type RawClient struct {
	relayLabel string
	cmgr       cmgr.Cmgr
	tCPRemotes lb.RoundRobin
	l          *zap.SugaredLogger
}

func newRawClient(relayLabel string, tcpRemotes lb.RoundRobin, cmgr cmgr.Cmgr) *RawClient {
	r := &RawClient{
		cmgr:       cmgr,
		relayLabel: relayLabel,
		tCPRemotes: tcpRemotes,
		l:          zap.S().Named(relayLabel),
	}
	return r
}

func (raw *RawClient) GetRemote() *lb.Node {
	return raw.tCPRemotes.Next()
}

func (raw *RawClient) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	d := net.Dialer{Timeout: constant.DialTimeOut}
	rc, err := d.Dial("tcp", remote.Address)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return rc, nil
}

func (raw *RawClient) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Inc()
	defer metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Dec()

	clonedRemote := remote.Clone()
	rc, err := raw.dialRemote(clonedRemote)
	if err != nil {
		return err
	}
	raw.l.Infof("HandleTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(raw.relayLabel, c, rc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	raw.cmgr.AddConnection(relayConn)
	defer raw.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

type RawServer struct {
	rtp RelayTransporter
	lis *net.TCPListener
	l   *zap.SugaredLogger
}

func NewRawServer(addr string, rtp RelayTransporter) (*RawServer, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	lis, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	return &RawServer{lis: lis, rtp: rtp}, nil
}

func (s *RawServer) Close() error {
	return s.lis.Close()
}

func (s *RawServer) ListenAndServe() error {
	for {
		c, err := s.lis.AcceptTCP()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			remote := s.rtp.GetRemote()
			metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Inc()
			defer metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Dec()
			if err := s.rtp.HandleTCPConn(c, remote); err != nil {
				s.l.Errorf("HandleTCPConn meet error tp:%s from:%s to:%s err:%s",
					s.rtp,
					c.RemoteAddr(), remote.Address, err)
			}
		}(c)
	}
}
