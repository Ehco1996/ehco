// nolint: errcheck
package transporter

import (
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var _ RelayTransporter = &RawClient{}

type RawClient struct {
	*baseTransporter

	dialer *net.Dialer
	lis    *net.TCPListener
}

func newRawClient(base *baseTransporter) (*RawClient, error) {
	localTCPAddr, err := net.ResolveTCPAddr("tcp", base.cfg.Listen)
	if err != nil {
		return nil, err
	}

	lis, err := net.ListenTCP("tcp", localTCPAddr)
	if err != nil {
		return nil, err
	}
	r := &RawClient{
		lis:             lis,
		baseTransporter: base,
		dialer:          &net.Dialer{Timeout: constant.DialTimeOut},
	}
	return r, nil
}

func (raw *RawClient) GetRemote() *lb.Node {
	return raw.baseTransporter.tCPRemotes.Next()
}

func (raw *RawClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	rc, err := raw.dialer.Dial("tcp", remote.Address)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return rc, nil
}

func (raw *RawClient) RelayTCPConn(c net.Conn) error {
	remote := raw.GetRemote()
	metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Inc()
	defer metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Dec()

	clonedRemote := remote.Clone()
	rc, err := raw.TCPHandShake(clonedRemote)
	if err != nil {
		return err
	}
	raw.l.Infof("RelayTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(
		raw.baseTransporter.cfg.Label, c, rc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	raw.cmgr.AddConnection(relayConn)
	defer raw.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

func (s *RawClient) Close() error {
	return s.lis.Close()
}

func (s *RawClient) ListenAndServe() error {
	for {
		c, err := s.lis.AcceptTCP()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			if err := s.RelayTCPConn(c); err != nil {
			}
		}(c)
	}
}
