package relay

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/transporter"
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
		// case Listen_WSS:
		// 	go func() {
		// 		errChan <- r.RunLocalWSSServer()
		// 	}()
		// case Listen_MWSS:
		// 	go func() {
		// 		errChan <- r.RunLocalMWSSServer()
		// }()
	}
	if len(r.cfg.UDPRemotes) > 0 {
		// 直接启动udp转发
		go func() {
			errChan <- r.RunLocalUDPServer()
		}()
	}
	return <-errChan
}

func (r *Relay) RunLocalTCPServer() error {
	logger.Logger.Infof("Start TCP relay At: %s Over: %s To: %s Through %s",
		r.LocalTCPAddr, r.ListenType, r.cfg.TCPRemotes, r.TransportType)

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

func index(w http.ResponseWriter, r *http.Request) {
	// TODO 加入一些链接比如 metrics pprof之类的
	logger.Logger.Infof("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func (r *Relay) RunLocalWSServer() error {

	// TODO 修一些取 HandleWebRequset的逻辑
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/ws/", r.TP.HandleWebRequset)
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
