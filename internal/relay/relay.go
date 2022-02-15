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
	"github.com/Ehco1996/ehco/internal/logger"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/transporter"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gorilla/mux"
	"go.uber.org/atomic"
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
			Address:    addr,
			Label:      fmt.Sprintf("%s-%s", cfg.Label, addr),
			BlockTimes: atomic.NewInt64(0),
		}
	}
	udpNodeList := make([]*lb.Node, len(cfg.UDPRemotes))
	for idx, addr := range cfg.UDPRemotes {
		udpNodeList[idx] = &lb.Node{
			Address:    addr,
			Label:      fmt.Sprintf("%s-%s", cfg.Label, addr),
			BlockTimes: atomic.NewInt64(0),
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
	switch r.ListenType {
	case constant.Listen_RAW:
		go func() {
			errCh <- r.RunLocalTCPServer()
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

	if len(r.cfg.UDPRemotes) > 0 {
		go func() {
			errCh <- r.RunLocalUDPServer()
		}()
	}
	return <-errCh
}

func (r *Relay) Close() {
	logger.Infof("[relay] Close relay %s", r.Name)
	if r.closeUdpF != nil {
		err := r.closeUdpF()
		if err != nil {
			logger.Errorf(err.Error())
		}
	}
	if r.closeTcpF != nil {
		err := r.closeTcpF()
		if err != nil {
			logger.Errorf(err.Error())
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
	logger.Infof("[relay] Start TCP relay %s", r.Name)
	for {
		c, err := lis.AcceptTCP()
		if err != nil {
			return err
		}
		if err := r.TP.LimitByIp(c); err != nil {
			logger.Errorf("reach tcp rate limit err:%s", c.RemoteAddr())
			c.Close()
			continue
		}

		go func(c *net.TCPConn) {
			remote := r.TP.GetRemote()
			web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Inc()
			defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TCP).Dec()
			defer c.Close()
			if err := r.TP.HandleTCPConn(c, remote); err != nil {
				logger.Errorf("HandleTCPConn meet error from:%s to:%s err:%s", c.RemoteAddr(), remote.Address, err)
			}
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
	logger.Infof("[relay] Start UDP relay %s", r.Name)

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

func (r *Relay) RunLocalWSServer() error {
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.Index)
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
	logger.Infof("[relay] Start WS relay %s", r.Name)
	return server.Serve(lis)
}

func (r *Relay) RunLocalWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.Index)
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
	logger.Infof("[relay] Start WSS relay %s", r.Name)
	return server.Serve(tls.NewListener(lis, server.TLSConfig))
}

func (r *Relay) RunLocalMWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mwssServer := transporter.NewMWSSServer()
	mux := mux.NewRouter()
	mux.Handle("/", http.HandlerFunc(web.Index))
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
	logger.Infof("[relay] Start MWSS relay %s", r.Name)
	go func() {
		err := httpServer.Serve(tls.NewListener(lis, httpServer.TLSConfig))
		if err != nil {
			mwssServer.ErrChan <- err
		}
		close(mwssServer.ErrChan)
	}()

	var tempDelay time.Duration
	for {
		conn, e := mwssServer.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				logger.Infof("server: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		tempDelay = 0
		go tp.HandleMWssRequest(conn)
	}
}
