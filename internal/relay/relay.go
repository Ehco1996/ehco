package relay

import (
	"crypto/tls"
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

	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	ListenType    string
	TransportType string

	TP transporter.RelayTransporter
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

	r := &Relay{
		cfg:           cfg,
		LocalTCPAddr:  localTCPAddr,
		LocalUDPAddr:  localUDPAddr,
		ListenType:    cfg.ListenType,
		TransportType: cfg.TransportType,

		TP: transporter.PickTransporter(
			cfg.TransportType,
			lb.NewRBRemotes(cfg.TCPRemotes),
			lb.NewRBRemotes(cfg.UDPRemotes),
		),
	}

	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errChan := make(chan error)

	switch r.ListenType {
	case constant.Listen_RAW:
		go func() {
			errChan <- r.RunLocalTCPServer()
		}()
	case constant.Listen_WS:
		go func() {
			errChan <- r.RunLocalWSServer()
		}()
	case constant.Listen_WSS:
		go func() {
			errChan <- r.RunLocalWSSServer()
		}()
	case constant.Listen_MWSS:
		go func() {
			errChan <- r.RunLocalMWSSServer()
		}()
	}
	if len(r.cfg.UDPRemotes) > 0 {
		// 直接启动udp转发
		go func() {
			errChan <- r.RunLocalUDPServer()
		}()
	}
	return <-errChan
}

func (r *Relay) LogRelay() {
	logger.Infof("[relay] TCP relay At: %s Over: %s To: %s Through %s",
		r.LocalTCPAddr, r.ListenType, r.cfg.TCPRemotes, r.TransportType)
}

func (r *Relay) RunLocalTCPServer() error {
	r.LogRelay()
	lis, err := net.ListenTCP("tcp", r.LocalTCPAddr)
	if err != nil {
		return err
	}
	defer lis.Close()
	for {
		c, err := lis.AcceptTCP()
		if err != nil {
			logger.Fatal("accept tcp conn error: %s", err)
		}
		go func(c *net.TCPConn) {
			if err := r.TP.HandleTCPConn(c); err != nil {
				logger.Infof("HandleTCPConn err %s", err)
			}
		}(c)
	}
}

func (r *Relay) RunLocalUDPServer() error {
	logger.Infof("[relay] Start UDP relay At: %s Over: %s To: %s Through %s",
		r.LocalTCPAddr, r.ListenType, r.cfg.UDPRemotes, r.TransportType)

	lis, err := net.ListenUDP("udp", r.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer lis.Close()

	buf := transporter.InboundBufferPool.Get().([]byte)
	defer transporter.InboundBufferPool.Put(buf)
	for {
		n, addr, err := lis.ReadFromUDP(buf)
		if err != nil {
			logger.Fatal("listen udp conn error: %s", err)
		}
		bc := r.TP.GetOrCreateBufferCh(addr)
		bc.Ch <- buf[0:n]
		if !bc.Handled {
			bc.Handled = true
			go r.TP.HandleUDPConn(addr, lis)
		}
	}
}

func (r *Relay) RunLocalWSServer() error {
	r.LogRelay()
	tp := r.TP.(*transporter.Raw)
	mux := mux.NewRouter()
	mux.HandleFunc("/", web.Index)
	mux.HandleFunc("/ws/", tp.HandleWsRequset)
	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()
	return server.Serve(ln)
}

func (r *Relay) RunLocalWSSServer() error {
	r.LogRelay()
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
	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()
	return server.Serve(tls.NewListener(ln, server.TLSConfig))
}

func (r *Relay) RunLocalMWSSServer() error {
	r.LogRelay()
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

	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	go func() {
		err := httpServer.Serve(tls.NewListener(ln, httpServer.TLSConfig))
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
