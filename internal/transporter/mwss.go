// nolint: errcheck
package transporter

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
	"github.com/gorilla/mux"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type Mwss struct {
	*Raw
	mtp *smuxTransporter
}

func (s *Mwss) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()
	t1 := time.Now()
	mwsc, err := s.mtp.Dial(context.TODO(), remote.Address+"/mwss/")
	web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	if err != nil {
		return err
	}
	defer mwsc.Close()
	s.L.Infof("HandleTCPConn from:%s to:%s", c.RemoteAddr(), remote.Address)
	return transport(c, mwsc, remote.Label)
}

type MWSSServer struct {
	raw        *Raw
	httpServer *http.Server
	L          *zap.SugaredLogger

	connChan chan net.Conn
	errChan  chan error
}

func NewMWSSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *MWSSServer {
	s := &MWSSServer{
		raw:      raw,
		L:        l,
		errChan:  make(chan error, 1),
		connChan: make(chan net.Conn, 1024),
	}

	mux := mux.NewRouter()
	mux.Handle("/", web.MakeIndexF(l))
	mux.Handle("/mwss/", http.HandlerFunc(s.HandleRequest))
	s.httpServer = &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
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
			defer c.Close()
			remote := s.raw.GetRemote()
			if err := s.raw.HandleTCPConn(c, remote); err != nil {
				s.L.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
		}(conn)
	}
}

func (s *MWSSServer) HandleRequest(w http.ResponseWriter, r *http.Request) {
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
		s.L.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.httpServer.Addr, err)
		return
	}
	defer session.Close()

	s.L.Debugf("session init %s  %s", conn.RemoteAddr(), s.httpServer.Addr)
	defer s.L.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.httpServer.Addr)

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

type MWSSClient struct {
	dialer *ws.Dialer
	l      *zap.SugaredLogger
}

func NewMWSSClient(l *zap.SugaredLogger) *MWSSClient {
	dialer := &ws.Dialer{
		TLSConfig: mytls.DefaultTLSConfig,
		Timeout:   constant.DialTimeOut,
	}

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
	c.l.Infof("init new session to: %s", rc.RemoteAddr())
	return session, nil
}
