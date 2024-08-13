// nolint: errcheck
package transporter

import (
	"context"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"go.uber.org/zap"
)

var (
	_ RelayClient = &RawClient{}
	_ RelayServer = &RawServer{}
)

type RawClient struct {
	dialer *net.Dialer
	cfg    *conf.Config
	l      *zap.SugaredLogger
}

func newRawClient(cfg *conf.Config) (*RawClient, error) {
	r := &RawClient{
		l:      zap.S().Named("raw"),
		cfg:    cfg,
		dialer: &net.Dialer{Timeout: constant.DialTimeOut},
	}
	r.dialer.SetMultipathTCP(true)
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

func (raw *RawClient) HealthCheck(ctx context.Context, remote *lb.Node) error {
	l := zap.S().Named("health-check")
	l.Infof("start send req to %s", remote.Address)
	c, err := raw.TCPHandShake(remote)
	if err != nil {
		l.Errorf("send req to %s meet error:%s", remote.Address, err)
		return err
	}
	c.Close()
	return nil
}

type RawServer struct {
	*baseTransporter
	lis net.Listener
}

func newRawServer(base *baseTransporter) (*RawServer, error) {
	addr, err := base.GetTCPListenAddr()
	if err != nil {
		return nil, err
	}
	cfg := net.ListenConfig{}
	cfg.SetMultipathTCP(true)
	lis, err := cfg.Listen(context.TODO(), "tcp", addr.String())
	if err != nil {
		return nil, err
	}
	return &RawServer{
		lis:             lis,
		baseTransporter: base,
	}, nil
}

func (s *RawServer) Close() error {
	return s.lis.Close()
}

func (s *RawServer) ListenAndServe() error {
	for {
		c, err := s.lis.Accept()
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
