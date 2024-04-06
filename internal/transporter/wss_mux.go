// nolint: errcheck
package transporter

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
	"github.com/xtaci/smux"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type MWSSClient struct {
	*RawClient
	dialer *ws.Dialer
	mtp    *smuxTransporter
}

func newMWSSClient(raw *RawClient) *MWSSClient {
	dialer := &ws.Dialer{TLSConfig: mytls.DefaultTLSConfig, Timeout: constant.DialTimeOut}
	c := &MWSSClient{dialer: dialer, RawClient: raw}
	mtp := NewSmuxTransporter(raw.l.Named("mwss"), c.initNewSession)
	c.mtp = mtp
	return c
}

func (c *MWSSClient) initNewSession(ctx context.Context, addr string) (*smux.Session, error) {
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
	c.l.Infof("init new session to: %s", rc.RemoteAddr())
	return session, nil
}

func (s *MWSSClient) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	mwssc, err := s.mtp.Dial(context.TODO(), remote.Address+"/handshake/")
	if err != nil {
		return nil, err
	}

	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return mwssc, nil
}

func (s *MWSSClient) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	clonedRemote := remote.Clone()
	mwsc, err := s.dialRemote(clonedRemote)
	if err != nil {
		return err
	}
	s.l.Infof("HandleTCPConn from:%s to:%s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(s.relayLabel, c, mwsc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	s.cmgr.AddConnection(relayConn)
	defer s.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

type MWSSServer struct {
	raw        *RawClient
	httpServer *http.Server
	l          *zap.SugaredLogger

	connChan chan net.Conn
	errChan  chan error
}

func NewMWSSServer(listenAddr string, raw *RawClient, l *zap.SugaredLogger) *MWSSServer {
	s := &MWSSServer{
		raw:      raw,
		l:        l,
		errChan:  make(chan error, 1),
		connChan: make(chan net.Conn, 1024),
	}

	e := web.NewEchoServer()
	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
	e.GET("/handshake/", echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
	s.httpServer = &http.Server{
		Addr:              listenAddr,
		Handler:           e,
		TLSConfig:         mytls.DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
	}
	return s
}

func (s *MWSSServer) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	go func() {
		s.errChan <- s.httpServer.Serve(tls.NewListener(lis, s.httpServer.TLSConfig))
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

func (s *MWSSServer) HandleRequest(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.l.Error(err)
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
		s.l.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.httpServer.Addr, err)
		return
	}
	defer session.Close()

	s.l.Debugf("session init %s  %s", conn.RemoteAddr(), s.httpServer.Addr)
	defer s.l.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.httpServer.Addr)

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

func (s *MWSSServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.connChan:
	case err = <-s.errChan:
	}
	return
}

func (s *MWSSServer) Close() error {
	return s.httpServer.Close()
}
