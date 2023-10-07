// nolint: errcheck
package transporter

import (
	"context"
	"net"
	"time"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type MTCP struct {
	*Raw
	mtp *smuxTransporter
}

func (s *MTCP) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()
	t1 := time.Now()
	mwsc, err := s.mtp.Dial(context.TODO(), remote.Address)
	web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	if err != nil {
		return err
	}
	defer mwsc.Close()
	s.L.Infof("HandleTCPConn from:%s to:%s", c.RemoteAddr(), remote.Address)
	return transport(c, mwsc, remote.Label)
}

type MTCPServer struct {
	raw        *Raw
	listenAddr string
	listener   net.Listener
	L          *zap.SugaredLogger

	errChan  chan error
	connChan chan net.Conn
}

func NewMTCPServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *MTCPServer {
	return &MTCPServer{
		L:          l,
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
		s.L.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.listenAddr, err)
		return
	}
	defer session.Close()

	s.L.Debugf("session init %s  %s", conn.RemoteAddr(), s.listenAddr)
	defer s.L.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.listenAddr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			s.L.Errorf("accept stream err: %s", err)
			break
		}
		select {
		case s.connChan <- stream:
		default:
			stream.Close()
			s.L.Infof("%s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
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
			defer c.Close()
			remote := s.raw.GetRemote()
			if err := s.raw.HandleTCPConn(c, remote); err != nil {
				s.L.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
		}(conn)
	}
}

func (s *MTCPServer) Close() error {
	return s.listener.Close()
}

type MTCPClient struct {
	l *zap.SugaredLogger
}

func NewMTCPClient(l *zap.SugaredLogger) *MTCPClient {
	return &MTCPClient{l: l}
}

func (c *MTCPClient) InitNewSession(ctx context.Context, addr string) (*smux.Session, error) {
	rc, err := net.Dial("tcp", addr)
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
