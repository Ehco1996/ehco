package transporter

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/web"
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
		dialer: &ws.Dialer{Timeout: cfg.Options.DialTimeout},
	}
	return s, nil
}

func (s *WsClient) addUDPQueryParam(addr string) string {
	u, err := url.Parse(addr)
	if err != nil {
		s.l.Errorf("Failed to parse URL: %v", err)
		return addr
	}
	q := u.Query()
	q.Set("type", "udp")
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *WsClient) HandShake(ctx context.Context, remote *lb.Node, isTCP bool) (net.Conn, error) {
	t1 := time.Now()
	addr := fmt.Sprintf("%s/handshake", remote.Address)
	if !isTCP {
		addr = s.addUDPQueryParam(addr)
	}
	wsc, _, _, err := s.dialer.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	connType := metrics.METRIC_CONN_TYPE_TCP
	if !isTCP {
		connType = metrics.METRIC_CONN_TYPE_UDP
	}
	labels := []string{s.cfg.Label, connType, remote.Address}
	metrics.HandShakeDurationMilliseconds.WithLabelValues(labels...).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	c := conn.NewWSConn(wsc, false)
	return c, nil
}

type WsServer struct {
	*BaseRelayServer
	httpServer *http.Server
}

func newWsServer(bs *BaseRelayServer) (*WsServer, error) {
	s := &WsServer{BaseRelayServer: bs}
	e := web.NewEchoServer()
	e.Use(web.NginxLogMiddleware(zap.S().Named("ws-server")))
	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
	e.GET("handshake", echo.WrapHandler(http.HandlerFunc(s.handleRequest)))
	s.httpServer = &http.Server{Handler: e}
	return s, nil
}

func (s *WsServer) handleRequest(w http.ResponseWriter, req *http.Request) {
	// todo use bufio.ReadWriter
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}

	remote := s.remotes.Next()
	if req.URL.Query().Get("type") == "udp" {
		if !s.cfg.Options.EnableUDP {
			s.l.Error("udp not support but request with udp type")
			wsc.Close()
			return
		}
		err = s.RelayUDPConn(req.Context(), conn.NewWSConn(wsc, true), remote)
	} else {
		err = s.RelayTCPConn(req.Context(), conn.NewWSConn(wsc, true), remote)
	}
	if err != nil {
		s.l.Errorf("handleRequest meet error:%s", err)
	}
}

func (s *WsServer) ListenAndServe(ctx context.Context) error {
	listener, err := NewTCPListener(ctx, s.cfg)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(listener)
}

func (s *WsServer) Close() error {
	return s.httpServer.Close()
}
