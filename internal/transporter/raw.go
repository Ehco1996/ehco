// nolint: errcheck
package transporter

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/conn"
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
		cfg:    cfg,
		dialer: NewNetDialer(cfg),
		l:      zap.S().Named(string(cfg.TransportType)),
	}
	return r, nil
}

func (raw *RawClient) HandShake(ctx context.Context, remote *lb.Node, isTCP bool) (net.Conn, error) {
	t1 := time.Now()
	var rc net.Conn
	var err error
	if isTCP {
		rc, err = raw.dialer.DialContext(ctx, "tcp", remote.Address)
	} else {
		rc, err = raw.dialer.DialContext(ctx, "udp", remote.Address)
	}
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return rc, nil
}

type RawServer struct {
	*BaseRelayServer

	tcpLis net.Listener
	udpLis *conn.UDPListener
}

func newRawServer(bs *BaseRelayServer) (*RawServer, error) {
	rs := &RawServer{BaseRelayServer: bs}

	return rs, nil
}

func (s *RawServer) Close() error {
	err := s.tcpLis.Close()
	if s.udpLis != nil {
		err2 := s.udpLis.Close()
		err = errors.Join(err, err2)
	}
	return err
}

func (s *RawServer) ListenAndServe(ctx context.Context) error {
	ts, err := NewTCPListener(ctx, s.cfg)
	if err != nil {
		return err
	}
	s.tcpLis = ts

	if s.cfg.Options != nil && s.cfg.Options.EnableUDP {
		udpLis, err := conn.NewUDPListener(ctx, s.cfg.Listen)
		if err != nil {
			return err
		}
		s.udpLis = udpLis
	}

	if s.udpLis != nil {
		go s.listenUDP(ctx)
	}
	for {
		c, err := s.tcpLis.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			defer c.Close()
			if err := s.RelayTCPConn(ctx, c); err != nil {
				s.l.Errorf("RelayTCPConn meet error: %s", err.Error())
			}
		}(c)
	}
}

func (s *RawServer) listenUDP(ctx context.Context) error {
	s.l.Infof("Start UDP server at %s", s.cfg.Listen)
	for {
		c, err := s.udpLis.Accept()
		if err != nil {
			s.l.Errorf("UDP accept error: %v", err)
			return err
		}
		go func() {
			if err := s.RelayUDPConn(ctx, c); err != nil {
				s.l.Errorf("RelayUDPConn meet error: %s", err.Error())
			}
		}()
	}
}
