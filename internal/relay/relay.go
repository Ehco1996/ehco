package relay

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/transporter"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

var doOnce sync.Once

type Relay struct {
	cfg *config.RelayConfig

	ListenType    string
	TransportType string
	LocalTCPAddr  *net.TCPAddr
	LocalUDPAddr  *net.UDPAddr
	TP            transporter.RelayTransporter

	closeTcpF func() error
	closeUdpF func() error

	Name string
	L    *zap.SugaredLogger
}

func NewRelay(cfg *config.RelayConfig) (*Relay, error) {
	localTCPAddr, err := net.ResolveTCPAddr("tcp", cfg.Listen)
	if err != nil {
		return nil, err
	}
	localUDPAddr, err := net.ResolveUDPAddr("udp", cfg.Listen)
	if err != nil {
		return nil, err
	}

	tcpNodeList := make([]*lb.Node, len(cfg.TCPRemotes))
	for idx, addr := range cfg.TCPRemotes {
		tcpNodeList[idx] = &lb.Node{
			Address: addr,
			Label:   fmt.Sprintf("%s-%s", cfg.Label, addr),
		}
	}
	udpNodeList := make([]*lb.Node, len(cfg.UDPRemotes))
	for idx, addr := range cfg.UDPRemotes {
		udpNodeList[idx] = &lb.Node{
			Address: addr,
			Label:   fmt.Sprintf("%s-%s", cfg.Label, addr),
		}
	}

	r := &Relay{
		cfg: cfg,

		LocalTCPAddr:  localTCPAddr,
		LocalUDPAddr:  localUDPAddr,
		ListenType:    cfg.ListenType,
		TransportType: cfg.TransportType,
		TP: transporter.PickTransporter(
			cfg.TransportType,
			lb.NewRoundRobin(tcpNodeList),
			lb.NewRoundRobin(udpNodeList),
		),
		L: log.Logger.Named("relay"),
	}
	r.Name = fmt.Sprintf("<At=%s Over=%s TCP-To=%s UDP-To=%s Through=%s>",
		r.LocalTCPAddr, r.ListenType, r.cfg.TCPRemotes, r.cfg.UDPRemotes, r.TransportType)
	return r, nil
}

func (r *Relay) ListenAndServe() error {
	doOnce.Do(func() {
		web.EhcoAlive.Set(web.EhcoAliveStateRunning)
	})

	errCh := make(chan error)

	if len(r.cfg.TCPRemotes) > 0 {
		switch r.ListenType {
		case constant.Listen_RAW:
			go func() {
				errCh <- r.RunLocalTCPServer()
			}()
		case constant.Listen_MTCP:
			go func() {
				errCh <- r.RunLocalMTCPServer()
			}()
		case constant.Listen_WS:
			go func() {
				errCh <- r.RunLocalWSServer()
			}()
		case constant.Listen_WSS:
			go func() {
				errCh <- r.RunLocalWSSServer()
			}()
		case constant.Listen_MWSS:
			go func() {
				errCh <- r.RunLocalMWSSServer()
			}()
		}
	}

	if len(r.cfg.UDPRemotes) > 0 {
		go func() {
			errCh <- r.RunLocalUDPServer()
		}()
	}
	return <-errCh
}

func (r *Relay) Close() {
	r.L.Infof("Close relay %s", r.Name)
	if r.closeUdpF != nil {
		err := r.closeUdpF()
		if err != nil {
			r.L.Errorf(err.Error())
		}
	}
	if r.closeTcpF != nil {
		err := r.closeTcpF()
		if err != nil {
			r.L.Errorf(err.Error())
		}
	}
}

func (r *Relay) RunLocalTCPServer() error {
	lis, err := net.ListenTCP("tcp", r.LocalTCPAddr)
	if err != nil {
		return err
	}
	defer lis.Close()
	r.closeTcpF = func() error {
		return lis.Close()
	}
	r.L.Infof("Start TCP relay Server %s", r.Name)
	for {
		c, err := lis.AcceptTCP()
		if err != nil {
			return err
		}

		go func(c net.Conn) {
			remote := r.TP.GetRemote()
			web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
			defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()
			defer c.Close()
			t1 := time.Now()
			if err := r.TP.HandleTCPConn(c, remote); err != nil {
				r.L.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
			web.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(time.Since(t1).Milliseconds()))
		}(c)
	}
}

func (r *Relay) RunLocalUDPServer() error {
	lis, err := net.ListenUDP("udp", r.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer lis.Close()
	r.closeUdpF = func() error {
		return lis.Close()
	}
	r.L.Infof("Start UDP relay Server %s", r.Name)

	buf := transporter.BufferPool.Get()
	defer transporter.BufferPool.Put(buf)
	for {
		n, addr, err := lis.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		bc := r.TP.GetOrCreateBufferCh(addr)
		bc.Ch <- buf[0:n]
		if !bc.Handled.Load() {
			bc.Handled.Store(true)
			go r.TP.HandleUDPConn(bc.UDPAddr, lis)
		}
	}
}

func (r *Relay) RunLocalMTCPServer() error {
	mTCPServer := transporter.NewMTCPServer(r.L.Named("MTCPServer"), r.LocalTCPAddr)
	r.closeTcpF = func() error {
		return mTCPServer.Close()
	}

	go func() {
		r.L.Infof("Start MTCP relay server %s", r.Name)
		mTCPServer.ListenAndServe()
	}()

	tp := r.TP.(*transporter.Raw)
	for {
		conn, e := mTCPServer.Accept()
		if e != nil {
			return e
		}
		go func(c net.Conn) {
			remote := tp.GetRemote()
			web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
			defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()
			defer c.Close()
			if err := tp.HandleTCPConn(c, remote); err != nil {
				r.L.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
		}(conn)
	}
}

func (r *Relay) RunLocalWSServer() error {
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF(r.L))
	mux.HandleFunc("/ws/", tp.HandleWsRequest)
	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	lis, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer lis.Close()
	r.closeTcpF = func() error {
		return lis.Close()
	}
	r.L.Infof("Start WS relay Server %s", r.Name)
	return server.Serve(lis)
}

func (r *Relay) RunLocalWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.MakeIndexF(r.L))
	mux.HandleFunc("/wss/", tp.HandleWssRequest)

	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		TLSConfig:         mytls.DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	lis, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer lis.Close()
	r.closeTcpF = func() error {
		return lis.Close()
	}
	r.L.Infof("Start WSS relay Server %s", r.Name)
	return server.Serve(tls.NewListener(lis, server.TLSConfig))
}

func (r *Relay) RunLocalMWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mwssServer := transporter.NewMWSSServer(r.L.Named("MWSSServer"))
	mux := mux.NewRouter()
	mux.Handle("/", web.MakeIndexF(r.L))
	mux.Handle("/mwss/", http.HandlerFunc(mwssServer.Upgrade))
	httpServer := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		Handler:           mux,
		TLSConfig:         mytls.DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
	}
	mwssServer.Server = httpServer

	lis, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer lis.Close()
	r.closeTcpF = func() error {
		return lis.Close()
	}
	r.L.Infof("Start MWSS relay Server %s", r.Name)
	go func() {
		err := httpServer.Serve(tls.NewListener(lis, httpServer.TLSConfig))
		if err != nil {
			mwssServer.ErrChan <- err
		}
		close(mwssServer.ErrChan)
	}()

	for {
		conn, e := mwssServer.Accept()
		if e != nil {
			return e
		}
		go tp.HandleMWssRequest(conn)
	}
}
