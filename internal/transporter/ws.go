package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &WsClient{}
	_ RelayServer = &WsServer{}
)

type WsClient struct {
	*baseTransporter

	dialer *ws.Dialer
}

func newWsClient(base *baseTransporter) (*WsClient, error) {
	s := &WsClient{
		baseTransporter: base,
		dialer:          &ws.Dialer{Timeout: constant.DialTimeOut},
	}
	return s, nil
}

func (s *WsClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	wsc, _, _, err := s.dialer.Dial(context.TODO(), remote.Address+"/handshake/")
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	c := newWsConn(wsc, false)
	return c, nil
}

type WsServer struct {
	*baseTransporter

	e          *echo.Echo
	httpServer *http.Server
	relayer    RelayClient
}

func newWsServer(base *baseTransporter) (*WsServer, error) {
	localTCPAddr, err := base.GetTCPListenAddr()
	if err != nil {
		return nil, err
	}
	s := &WsServer{
		baseTransporter: base,
		httpServer: &http.Server{
			Addr: localTCPAddr.String(), ReadHeaderTimeout: 30 * time.Second,
		},
	}
	e := web.NewEchoServer()
	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
	e.GET("/handshake/", echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
	s.e = e
	relayer, err := NewRelayClient(base.cfg.TransportType, base)
	if err != nil {
		return nil, err
	}
	s.relayer = relayer
	return s, nil
}

func (s *WsServer) ListenAndServe() error {
	return s.e.StartServer(s.httpServer)
}

func (s *WsServer) Close() error {
	return s.e.Close()
}

func (s *WsServer) HandleRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	c := newWsConn(wsc, true)
	if err := s.RelayTCPConn(c, s.relayer.TCPHandShake); err != nil {
		s.l.Errorf("RelayTCPConn error: %s", err.Error())
	}
}
