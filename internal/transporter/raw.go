// nolint: errcheck
package transporter

import (
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var _ RelayTransporter = &RawClient{}

type RawClient struct {
	*baseTransporter

	dialer       *net.Dialer
	localTCPAddr *net.TCPAddr
	lis          *net.TCPListener
}

func newRawClient(base *baseTransporter) (*RawClient, error) {
	localTCPAddr, err := base.GetTCPListenAddr()
	if err != nil {
		return nil, err
	}
	r := &RawClient{
		baseTransporter: base,
		localTCPAddr:    localTCPAddr,
		dialer:          &net.Dialer{Timeout: constant.DialTimeOut},
	}
	return r, nil
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

func (s *RawClient) Close() error {
	return s.lis.Close()
}

func (s *RawClient) ListenAndServe() error {
	lis, err := net.ListenTCP("tcp", s.localTCPAddr)
	if err != nil {
		return err
	}
	s.lis = lis
	tp, err := NewRelayTransporter(s.cfg.TransportType, s.baseTransporter)
	if err != nil {
		return err
	}
	for {
		c, err := s.lis.AcceptTCP()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			if err := s.baseTransporter.RelayTCPConn(c, tp.TCPHandShake); err != nil {
				s.l.Errorf("RelayTCPConn error: %s", err.Error())
			}
		}(c)
	}
}
