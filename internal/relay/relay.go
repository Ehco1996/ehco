package relay

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/transporter"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gorilla/mux"
)

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
	}
	r.Name = fmt.Sprintf("[At=%s Over=%s To=%s Through=%s]",
		r.LocalTCPAddr, r.ListenType, r.cfg.TCPRemotes, r.TransportType)
	return r, nil
}

func (r *Relay) ListenAndServe() error {
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
		go func(c *net.TCPConn) {
			if err := r.TP.HandleTCPConn(c); err != nil {
				logger.Errorf("HandleTCPConn err=%s name=%s", err, r.Name)
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
	mux.HandleFunc("/ws/", tp.HandleWsRequset)
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
	return server.Serve(lis)
}

func (r *Relay) RunLocalWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.Index)
	mux.HandleFunc("/wss/", tp.HandleWssRequset)

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
		go tp.HandleMWssRequset(conn)
	}
}
