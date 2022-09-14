package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type MTCP struct {
	raw *Raw
	mtp *smuxTransporter
}

func (s *MTCP) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *MTCP) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *MTCP) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()
	mwsc, err := s.mtp.Dial(context.TODO(), remote.Address)
	if err != nil {
		return err
	}
	defer mwsc.Close()
	s.raw.L.Infof("HandleTCPConn from:%s to:%s", c.RemoteAddr(), remote.Address)
	return transport(c, mwsc, remote.Label)
}

func (s *MTCP) GetRemote() *lb.Node {
	return s.raw.GetRemote()
}

type MTCPServer struct {
	listenAddr *net.TCPAddr
	listener   *net.TCPListener

	ConnChan chan net.Conn
	ErrChan  chan error
	L        *zap.SugaredLogger
}

func NewMTCPServer(l *zap.SugaredLogger, listenAddr *net.TCPAddr) *MTCPServer {
	return &MTCPServer{
		ConnChan:   make(chan net.Conn, 1024),
		ErrChan:    make(chan error, 1),
		L:          l,
		listenAddr: listenAddr,
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
		case s.ConnChan <- stream:
		default:
			stream.Close()
			s.L.Infof("%s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}

func (s *MTCPServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.ConnChan:
	case err = <-s.ErrChan:
	}
	return
}

func (s *MTCPServer) ListenAndServe() {
	lis, err := net.ListenTCP("tcp", s.listenAddr)
	if err != nil {
		s.ErrChan <- err
		return
	}
	s.listener = lis
	for {
		c, err := lis.AcceptTCP()
		if err != nil {
			s.ErrChan <- err
			continue
		}

		go s.mux(c)
	}

}

func (s *MTCPServer) Close() error {
	return s.listener.Close()
}

type MTCPClient struct {
	l *zap.SugaredLogger
}

func NewMTCPClient(l *zap.SugaredLogger) *MTCPClient {
	return &MTCPClient{
		l: l.Named("MTCPClient"),
	}
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
	c.l.Infof("Init new session to: %s", rc.RemoteAddr())
	return session, nil
}
