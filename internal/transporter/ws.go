package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type Ws struct {
	*Raw
}

func (s *Ws) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	d := ws.Dialer{Timeout: constant.DialTimeOut}
	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/ws/")
	if err != nil {
		return nil, err
	}
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
	return wsc, nil
}

func (s *Ws) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	wsc, err := s.dialRemote(remote)
	if err != nil {
		return err
	}
	s.l.Infof("HandleTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(s.relayLabel, c, wsc)
	s.cmgr.AddConnection(relayConn)
	defer s.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}

type WSServer struct {
	raw        *Raw
	l          *zap.SugaredLogger
	httpServer *http.Server
}

func NewWSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *WSServer {
	s := &WSServer{raw: raw, l: l}
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF())
	mux.HandleFunc("/ws/", s.HandleRequest)
	s.httpServer = &http.Server{
		Addr:              listenAddr,
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	return s
}

func (s *WSServer) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *WSServer) Close() error {
	return s.httpServer.Close()
}

func (s *WSServer) HandleRequest(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}

	remote := s.raw.GetRemote()
	if err := s.raw.HandleTCPConn(wsc, remote); err != nil {
		s.l.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err)
	}
}
