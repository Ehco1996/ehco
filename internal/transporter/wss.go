// nolint: errcheck
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
	"github.com/Ehco1996/ehco/internal/metrics"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type Wss struct {
	*Raw
}

func (s *Wss) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	d := ws.Dialer{TLSConfig: mytls.DefaultTLSConfig}
	wssc, _, _, err := d.Dial(context.TODO(), remote.Address+"/wss/")
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return wssc, nil
}

func (s *Wss) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	clonedRemote := remote.Clone()
	wssc, err := s.dialRemote(clonedRemote)
	if err != nil {
		return err
	}
	s.l.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)

	relayConn := conn.NewRelayConn(s.relayLabel, c, wssc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	s.cmgr.AddConnection(relayConn)
	defer s.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

type WSSServer struct{ WSServer }

func NewWSSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *WSSServer {
	wsServer := NewWSServer(listenAddr, raw, l)
	wsServer.e.GET("/wss/", echo.WrapHandler(http.HandlerFunc(wsServer.HandleRequest)))
	return &WSSServer{WSServer: *wsServer}
}

func (s *WSSServer) ListenAndServe() error {
	s.httpServer.TLSConfig = mytls.DefaultTLSConfig
	return s.WSServer.ListenAndServe()
}
