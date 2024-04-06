// nolint: errcheck
package transporter

import (
	"context"
	"net"
	"time"

	"github.com/xtaci/smux"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type MTCPClient struct {
	*RawClient
	dialer *net.Dialer
	mtp    *smuxTransporter
}

func newMTCPClient(raw *RawClient) *MTCPClient {
	dialer := &net.Dialer{Timeout: constant.DialTimeOut}
	c := &MTCPClient{dialer: dialer, RawClient: raw}
	mtp := NewSmuxTransporter(raw.l.Named("mtcp"), c.initNewSession)
	c.mtp = mtp
	return c
}

func (c *MTCPClient) initNewSession(ctx context.Context, addr string) (*smux.Session, error) {
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

func (s *MTCPClient) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	mtcpc, err := s.mtp.Dial(context.TODO(), remote.Address)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return mtcpc, nil
}

func (s *MTCPClient) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	clonedRemote := remote.Clone()
	mtcpc, err := s.dialRemote(clonedRemote)
	if err != nil {
		return err
	}
	s.l.Infof("HandleTCPConn from:%s to:%s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(s.relayLabel, c, mtcpc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	s.cmgr.AddConnection(relayConn)
	defer s.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

type MTCPServer struct {
	raw        *RawClient
	listenAddr string
	listener   net.Listener
	l          *zap.SugaredLogger

	errChan  chan error
	connChan chan net.Conn
}

func NewMTCPServer(listenAddr string, raw *RawClient, l *zap.SugaredLogger) *MTCPServer {
	return &MTCPServer{
		l:          l,
		raw:        raw,
		listenAddr: listenAddr,
		errChan:    make(chan error, 1),
		connChan:   make(chan net.Conn, 1024),
	}
}

func (s *MTCPServer) mux(conn net.Conn) {
	defer conn.Close()

	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Server(conn, cfg)
	if err != nil {
		s.l.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.listenAddr, err)
		return
	}
	defer session.Close()

	s.l.Debugf("session init %s  %s", conn.RemoteAddr(), s.listenAddr)
	defer s.l.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.listenAddr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			s.l.Errorf("accept stream err: %s", err)
			break
		}
		select {
		case s.connChan <- stream:
		default:
			stream.Close()
			s.l.Infof("%s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}

func (s *MTCPServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.connChan:
	case err = <-s.errChan:
	}
	return
}

func (s *MTCPServer) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	s.listener = lis

	go func() {
		for {
			c, err := lis.Accept()
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
			remote := s.raw.GetRemote()
			if err := s.raw.HandleTCPConn(c, remote); err != nil {
				s.l.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
		}(conn)
	}
}

func (s *MTCPServer) Close() error {
	return s.listener.Close()
}
