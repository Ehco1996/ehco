package transporter

import (
	"context"
	"net"
	"net/http"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/gobwas/ws"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type Mwss struct {
	raw *Raw
	mtp *smuxTransporter
}

func (s *Mwss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Mwss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Mwss) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()
	mwsc, err := s.mtp.Dial(context.TODO(), remote.Address+"/mwss/")
	if err != nil {
		return err
	}
	defer mwsc.Close()
	s.raw.L.Infof("HandleTCPConn from:%s to:%s", c.RemoteAddr(), remote.Address)
	return transport(c, mwsc, remote.Label)
}

func (s *Mwss) GetRemote() *lb.Node {
	return s.raw.GetRemote()
}

type MWSSServer struct {
	Server   *http.Server
	ConnChan chan net.Conn
	ErrChan  chan error
	L        *zap.SugaredLogger
}

func NewMWSSServer(l *zap.SugaredLogger) *MWSSServer {
	return &MWSSServer{
		ConnChan: make(chan net.Conn, 1024),
		ErrChan:  make(chan error, 1),
		L:        l,
	}
}

func (s *MWSSServer) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.L.Error(err)
		return
	}
	s.mux(conn)
}

func (s *MWSSServer) mux(conn net.Conn) {
	defer conn.Close()

	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Server(conn, cfg)
	if err != nil {
		s.L.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.Server.Addr, err)
		return
	}
	defer session.Close()

	s.L.Debugf("session init %s  %s", conn.RemoteAddr(), s.Server.Addr)
	defer s.L.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.Server.Addr)

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

func (s *MWSSServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.ConnChan:
	case err = <-s.ErrChan:
	}
	return
}

func (s *MWSSServer) Close() error {
	return s.Server.Close()
}

type MWSSClient struct {
	dialer *ws.Dialer
	l      *zap.SugaredLogger
}

func NewMWSSClient(l *zap.SugaredLogger) *MWSSClient {
	dialer := &ws.Dialer{
		TLSConfig: mytls.DefaultTLSConfig,
		Timeout:   constant.DialTimeOut}

	return &MWSSClient{
		dialer: dialer,
		l:      l,
	}
}

func (c *MWSSClient) InitNewSession(ctx context.Context, addr string) (*smux.Session, error) {
	rc, _, _, err := c.dialer.Dial(ctx, addr)
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
