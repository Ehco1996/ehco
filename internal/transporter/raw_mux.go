package transporter

import (
	"context"
	"net"
	"time"

	"github.com/xtaci/smux"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &MtcpClient{}
	_ RelayServer = &MtcpServer{}
)

type MtcpClient struct {
	*RawClient
	muxTP *smuxTransporter
}

func newMtcpClient(cfg *conf.Config) (*MtcpClient, error) {
	raw, err := newRawClient(cfg)
	if err != nil {
		return nil, err
	}
	c := &MtcpClient{RawClient: raw}
	c.muxTP = NewSmuxTransporter(zap.S().Named("mtcp"), c.initNewSession)
	return c, nil
}

func (c *MtcpClient) initNewSession(ctx context.Context, addr string) (*smux.Session, error) {
	rc, err := c.dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	// stream multiplex
	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Client(rc, cfg)
	if err != nil {
		return nil, err
	}
	c.l.Infof("init new session to: %s", rc.RemoteAddr())
	return session, nil
}

func (s *MtcpClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	mtcpc, err := s.muxTP.Dial(context.TODO(), remote.Address)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return mtcpc, nil
}

type MtcpServer struct {
	*RawServer
	*muxServerImpl
}

func newMtcpServer(base *baseTransporter) (*MtcpServer, error) {
	raw, err := newRawServer(base)
	if err != nil {
		return nil, err
	}
	s := &MtcpServer{
		RawServer:     raw,
		muxServerImpl: newMuxServer(base.cfg.Listen, base.l.Named("mtcp")),
	}

	return s, nil
}

func (s *MtcpServer) ListenAndServe() error {
	go func() {
		for {
			c, err := s.lis.Accept()
			if err != nil {
				s.errChan <- err
				continue
			}
			go s.mux(c)
		}
	}()

	for {
		conn, e := s.Accept()
		if e != nil {
			return e
		}
		go func(c net.Conn) {
			if err := s.RelayTCPConn(c, s.relayer.TCPHandShake); err != nil {
				s.l.Errorf("RelayTCPConn error: %s", err.Error())
			}
		}(conn)
	}
}

func (s *MtcpServer) Close() error {
	return s.lis.Close()
}
