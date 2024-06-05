// nolint: errcheck
package transporter

import (
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &RawClient{}
	_ RelayServer = &RawServer{}
)

type RawClient struct {
	*baseTransporter

	dialer *net.Dialer
}

func newRawClient(base *baseTransporter) (*RawClient, error) {
	r := &RawClient{
		baseTransporter: base,
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

type RawServer struct {
	*baseTransporter
	localTCPAddr *net.TCPAddr
	lis          *net.TCPListener
	relayer      RelayClient
}

func newRawServer(base *baseTransporter) (*RawServer, error) {
	addr, err := base.GetTCPListenAddr()
	if err != nil {
		return nil, err
	}
	lis, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	relayer, err := newRelayClient(base)
	if err != nil {
		return nil, err
	}
	return &RawServer{
		lis:             lis,
		baseTransporter: base,
		localTCPAddr:    addr,
		relayer:         relayer,
	}, nil
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
			defer c.Close()
			if err := s.RelayTCPConn(c, s.relayer.TCPHandShake); err != nil {
				s.l.Errorf("RelayTCPConn error: %s", err.Error())
			}
		}(c)
	}
}
