package relay

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/transporter"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gorilla/mux"
)

type Relay struct {
	cfg *RelayConfig

	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	ListenType    string
	TransportType string

	TP transporter.RelayTransporter
}

func NewRelay(cfg *RelayConfig) (*Relay, error) {
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
			lb.New(cfg.TCPRemotes),
			lb.New(cfg.UDPRemotes),
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
	logger.Logger.Infof("Start TCP relay At: %s Over: %s To: %s Through %s",
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
			logger.Logger.Infof("accept tcp con error: %s", err)
			return err
		}
		if err := c.SetDeadline(time.Now().Add(constant.MaxConKeepAlive)); err != nil {
			logger.Logger.Infof("set max deadline err: %s", err)
			return err
		}
		go func(c *net.TCPConn) {
			if err := r.TP.HandleTCPConn(c); err != nil {
				logger.Logger.Infof("HandleTCPConn err %s", err)
			}
		}(c)
	}
}

func (r *Relay) RunLocalUDPServer() error {
	logger.Logger.Infof("Start UDP relay At: %s Over: %s To: %s Through %s",
		r.LocalTCPAddr, r.ListenType, r.cfg.UDPRemotes, r.TransportType)

	lis, err := net.ListenUDP("udp", r.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer lis.Close()

	buffers := transporter.NewBufferPool(constant.BUFFER_SIZE)
	buf := buffers.Get().([]byte)
	defer buffers.Put(buf)
	for {
		n, addr, err := lis.ReadFromUDP(buf)
		if err != nil {
			return err
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
	s := &transporter.MWSSServer{
		ConnChan: make(chan net.Conn, 1024),
		ErrChan:  make(chan error, 1),
	}
	mux := mux.NewRouter()
	mux.Handle("/", http.HandlerFunc(web.Index))
	mux.Handle("/mwss/", http.HandlerFunc(s.Upgrade))
	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		Handler:           mux,
		TLSConfig:         mytls.DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
	}
	s.Server = server

	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	go func() {
		err := server.Serve(tls.NewListener(ln, server.TLSConfig))
		if err != nil {
			s.ErrChan <- err
		}
		close(s.ErrChan)
	}()
	var tempDelay time.Duration
	for {
		conn, e := s.Accept()
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
				logger.Logger.Infof("server: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		tempDelay = 0
		go tp.HandleMWssRequset(conn)
	}
}
