package transporter

// import (
// 	"context"
// 	"net"
// 	"net/http"
// 	"time"

// 	"github.com/gobwas/ws"
// 	"github.com/labstack/echo/v4"
// 	"go.uber.org/zap"

// 	"github.com/Ehco1996/ehco/internal/conn"
// 	"github.com/Ehco1996/ehco/internal/constant"
// 	"github.com/Ehco1996/ehco/internal/metrics"
// 	"github.com/Ehco1996/ehco/internal/web"
// 	"github.com/Ehco1996/ehco/pkg/lb"
// )

// var _ RelayTransporter = &WsClient{}

// type WsClient struct {
// }

// func newWsClient(raw *RawClient) *WsClient {
// 	return &WsClient{RawClient: raw}
// }

// func (s *WsClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
// 	t1 := time.Now()
// 	d := ws.Dialer{Timeout: constant.DialTimeOut}
// 	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/handshake/")
// 	if err != nil {
// 		return nil, err
// 	}
// 	latency := time.Since(t1)
// 	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
// 	remote.HandShakeDuration = latency
// 	return wsc, nil
// }

// func (s *WsClient) RelayTCPConn(c net.Conn) error {
// 	remote := s.GetRemote()
// 	clonedRemote := remote.Clone()
// 	wsc, err := s.TCPHandShake(clonedRemote)
// 	if err != nil {
// 		return err
// 	}
// 	s.l.Infof("RelayTCPConn from %s to %s", c.LocalAddr(), remote.Address)
// 	relayConn := conn.NewRelayConn(
// 		s.relayLabel, c, wsc,
// 		conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
// 	s.cmgr.AddConnection(relayConn)
// 	defer s.cmgr.RemoveConnection(relayConn)
// 	return relayConn.Transport(remote.Label)
// }

// type WSServer struct {
// 	raw        *RawClient
// 	e          *echo.Echo
// 	httpServer *http.Server
// 	l          *zap.SugaredLogger
// }

// func NewWSServer(listenAddr string, raw *RawClient) *WSServer {
// 	s := &WSServer{
// 		l:          l,
// 		raw:        raw,
// 		httpServer: &http.Server{Addr: listenAddr, ReadHeaderTimeout: 30 * time.Second},
// 	}
// 	e := web.NewEchoServer()
// 	e.GET("/", echo.WrapHandler(web.MakeIndexF()))
// 	e.GET("/handshake/", echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
// 	s.e = e
// 	s.httpServer.Handler = e
// 	return s
// }

// func (s *WSServer) ListenAndServe() error {
// 	return s.e.StartServer(s.httpServer)
// }

// func (s *WSServer) Close() error {
// 	return s.e.Close()
// }

// func (s *WSServer) HandleRequest(w http.ResponseWriter, req *http.Request) {
// 	wsc, _, _, err := ws.UpgradeHTTP(req, w)
// 	if err != nil {
// 		return
// 	}
// 	if err := s.raw.RelayTCPConn(wsc); err != nil {
// 		s.l.Errorf("RelayTCPConn meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err)
// 	}
// }
