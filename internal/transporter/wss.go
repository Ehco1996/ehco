// nolint: errcheck
package transporter

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/lb"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type Wss struct {
	*Raw
}

func (s *Wss) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()

	d := ws.Dialer{TLSConfig: mytls.DefaultTLSConfig}
	t1 := time.Now()
	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/wss/")
	web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	if err != nil {
		return err
	}
	defer wsc.Close()
	s.L.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	return transport(c, wsc, remote.Label)
}

type WSSServer struct {
	raw        *Raw
	L          *zap.SugaredLogger
	httpServer *http.Server
}

func NewWSSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *WSSServer {
	s := &WSSServer{raw: raw, L: l}
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF(l))
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
	defer wsc.Close()

	remote := s.raw.GetRemote()
	if err := s.raw.HandleTCPConn(wsc, remote); err != nil {
		s.L.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err)
	}
}
