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
	raw *Raw
}

func (s *Wss) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Wss) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Wss) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	defer c.Close()

	d := ws.Dialer{TLSConfig: mytls.DefaultTLSConfig}
	wsc, _, _, err := d.Dial(context.TODO(), remote.Address+"/wss/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	s.raw.L.Infof("HandleTCPConn from %s to %s", c.RemoteAddr(), remote.Address)
	return transport(c, wsc, remote.Label)
}

func (s *Wss) GetRemote() *lb.Node {
	return s.raw.GetRemote()
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
		s.L.Infof("HandleRequest meet error from:%s to:%s err:%s", wsc.LocalAddr(), remote.Label, err.Error())
	}
}
