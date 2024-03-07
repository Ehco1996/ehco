// nolint: errcheck
package transporter

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/metrics"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
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

type WSSServer struct {
	raw        *Raw
	l          *zap.SugaredLogger
	httpServer *http.Server
}

func NewWSSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *WSSServer {
	s := &WSSServer{raw: raw, l: l}
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF())
	mux.HandleFunc("/wss/", s.HandleRequest)

	s.httpServer = &http.Server{
		Handler:           mux,
		Addr:              listenAddr,
		ReadHeaderTimeout: 30 * time.Second,
		TLSConfig:         mytls.DefaultTLSConfig,
	}
	return s
}

func (s *WSSServer) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	defer lis.Close()
	return s.httpServer.Serve(tls.NewListener(lis, s.httpServer.TLSConfig))
}

func (s *WSSServer) Close() error {
	return s.httpServer.Close()
}

func (s *WSSServer) HandleRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}

	remote := s.raw.GetRemote()
	if err := s.raw.HandleTCPConn(wsc, remote); err != nil {
		s.l.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err)
	}
}
