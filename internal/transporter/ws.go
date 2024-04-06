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

var _ RelayTransporter = &WsClient{}

type WsClient struct {
	*baseTransporter

	e          *echo.Echo
	dialer     *ws.Dialer
	httpServer *http.Server
	tp         RelayTransporter
}

func newWsClient(base *baseTransporter) (*WsClient, error) {
	localTCPAddr, err := base.GetTCPListenAddr()
	if err != nil {
		return nil, err
	}
	s := &WsClient{
		baseTransporter: base,
		httpServer: &http.Server{
			Addr: localTCPAddr.String(), ReadHeaderTimeout: 30 * time.Second},
		dialer: &ws.Dialer{Timeout: constant.DialTimeOut},
	}
	e := web.NewEchoServer()
	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
	e.GET("/handshake/", echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
	s.e = e
	s.httpServer.Handler = e
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
	return wsc, nil
}

func (s *WsClient) ListenAndServe() error {
	tp, err := NewRelayTransporter(s.cfg.TransportType, s.baseTransporter)
	if err != nil {
		return err
	}
	s.tp = tp
	return s.e.StartServer(s.httpServer)
}

func (s *WsClient) Close() error {
	return s.e.Close()
}

func (s *WsClient) HandleRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	if err := s.baseTransporter.RelayTCPConn(wsc, s.tp.TCPHandShake); err != nil {
		s.l.Errorf("RelayTCPConn error: %s", err.Error())
	}
}
