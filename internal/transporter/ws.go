package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &WsClient{}
	_ RelayServer = &WsServer{}
)

type WsClient struct {
	dialer *ws.Dialer
	cfg    *conf.Config
	l      *zap.SugaredLogger
}

func newWsClient(cfg *conf.Config) (*WsClient, error) {
	s := &WsClient{
		cfg:    cfg,
		l:      zap.S().Named(string(cfg.TransportType)),
		dialer: &ws.Dialer{Timeout: constant.DialTimeOut},
	}
	return s, nil
}

func (s *WsClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	addr, err := s.cfg.GetWSRemoteAddr(remote.Address)
	if err != nil {
		return nil, err
	}
	wsc, _, _, err := s.dialer.Dial(context.TODO(), addr)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	c := conn.NewWSConn(wsc, false)
	return c, nil
}

func (s *WsClient) HealthCheck(ctx context.Context, remote *lb.Node) error {
	l := zap.S().Named("health-check")
	l.Infof("start send req to %s", remote.Address)
	c, err := s.TCPHandShake(remote)
	if err != nil {
		l.Errorf("send req to %s meet error:%s", remote.Address, err)
		return err
	}
	c.Close()
	return nil
}

type WsServer struct {
	*baseTransporter

	e          *echo.Echo
	httpServer *http.Server
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
	e.Use(web.NginxLogMiddleware(zap.S().Named("ws-server")))

	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
	e.GET(base.cfg.GetWSHandShakePath(), echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))

	s.e = e
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

	if err := s.RelayTCPConn(conn.NewWSConn(wsc, true), s.relayer.TCPHandShake); err != nil {
		s.l.Errorf("RelayTCPConn error: %s", err.Error())
	}
}
