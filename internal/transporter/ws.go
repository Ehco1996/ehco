package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type Ws struct {
	raw *Raw
}

func (s *Ws) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Ws) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Ws) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()

	wsc, _, _, err := ws.Dial(context.TODO(), remote.Address+"/ws/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	s.raw.L.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	return transport(c, wsc, remote.Label)
}

func (s *Ws) GetRemote() *lb.Node {
	return s.raw.GetRemote()
}

type WSServer struct {
	raw        *Raw
	L          *zap.SugaredLogger
	httpServer *http.Server
}

func NewWSServer(listenAddr string, raw *Raw, l *zap.SugaredLogger) *WSServer {
	s := &WSServer{raw: raw, L: l}
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF(l))
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
	defer wsc.Close()
	remote := s.raw.TCPRemotes.Next()
	web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
	defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()
	rc, err := s.raw.DialRemote(remote)
	if err != nil {
		return
	}
	defer rc.Close()
	s.L.Infof("HandleRequest from:%s to:%s", wsc.RemoteAddr(), remote.Address)
	if err := transport(rc, wsc, remote.Label); err != nil {
		s.L.Infof("HandleRequest meet error from:%s to:%s err:%s", wsc.RemoteAddr(), remote.Address, err.Error())
	}
}
