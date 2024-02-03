package relay

import (
	"fmt"
	"net"

	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/transporter"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type Relay struct {
	Name          string // unique name for all relay\
	TransportType string
	ListenType    string
	TP            transporter.RelayTransporter

	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	closeTcpF func() error
	closeUdpF func() error
	cfg       *conf.Config
	l         *zap.SugaredLogger

	cmgr cmgr.Cmgr // register when start
}

func NewRelay(cfg *conf.Config) (*Relay, error) {
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
		l:   zap.S().Named("relay"),

		Name:          cfg.Label,
		LocalTCPAddr:  localTCPAddr,
		LocalUDPAddr:  localUDPAddr,
		ListenType:    cfg.ListenType,
		TransportType: cfg.TransportType,
		TP: transporter.NewRelayTransporter(
			cfg.TransportType,
			lb.NewRoundRobin(tcpNodeList),
			lb.NewRoundRobin(udpNodeList),
		),
	}

	return r, nil
}

func (r *Relay) registerMgr(cmgr cmgr.Cmgr) {
	r.cmgr = cmgr
}

func (r *Relay) ListenAndServe() error {
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
	r.l.Infof("Close relay label: %s", r.Name)
	if r.closeUdpF != nil {
		err := r.closeUdpF()
		if err != nil {
			r.l.Errorf(err.Error())
		}
	}
	if r.closeTcpF != nil {
		err := r.closeTcpF()
		if err != nil {
			r.l.Errorf(err.Error())
		}
	}
}

func (r *Relay) RunLocalTCPServer() error {
	lis, err := net.ListenTCP("tcp", r.LocalTCPAddr)
	if err != nil {
		return err
	}
	defer lis.Close() //nolint: errcheck
	r.closeTcpF = func() error {
		return lis.Close()
	}
	r.l.Infof("Start TCP relay Server: %s", r.Name)
	for {
		c, err := lis.AcceptTCP()
		if err != nil {
			return err
		}

		go func(c net.Conn) {
			remote := r.TP.GetRemote()
			web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Inc()
			defer web.CurConnectionCount.WithLabelValues(remote.Label, web.METRIC_CONN_TYPE_TCP).Dec()
			if err := r.TP.HandleTCPConn(c, remote); err != nil {
				r.l.Errorf("HandleTCPConn meet error tp:%s from:%s to:%s err:%s",
					r.TransportType,
					c.RemoteAddr(), remote.Address, err)
			}
		}(c)
	}
}

func (r *Relay) RunLocalUDPServer() error {
	lis, err := net.ListenUDP("udp", r.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer lis.Close() //nolint: errcheck
	r.closeUdpF = func() error {
		return lis.Close()
	}
	r.l.Infof("Start UDP relay Server: %s", r.Name)

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
	tp := r.TP.(*transporter.Raw)
	mTCPServer := transporter.NewMTCPServer(r.LocalTCPAddr.String(), tp, r.l.Named("MTCPServer"))
	r.closeTcpF = func() error {
		return mTCPServer.Close()
	}
	r.l.Infof("Start MTCP relay Server: %s", r.Name)
	return mTCPServer.ListenAndServe()
}

func (r *Relay) RunLocalWSServer() error {
	tp := r.TP.(*transporter.Raw)
	wsServer := transporter.NewWSServer(r.LocalTCPAddr.String(), tp, r.l.Named("WSServer"))
	r.closeTcpF = func() error {
		return wsServer.Close()
	}
	r.l.Infof("Start WS relay Server: %s", r.Name)
	return wsServer.ListenAndServe()
}

func (r *Relay) RunLocalWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	wssServer := transporter.NewWSSServer(r.LocalTCPAddr.String(), tp, r.l.Named("NewWSSServer"))
	r.closeTcpF = func() error {
		return wssServer.Close()
	}
	r.l.Infof("Start WSS relay Server: %s", r.Name)
	return wssServer.ListenAndServe()
}

func (r *Relay) RunLocalMWSSServer() error {
	tp := r.TP.(*transporter.Raw)
	mwssServer := transporter.NewMWSSServer(r.LocalTCPAddr.String(), tp, r.l.Named("MWSSServer"))
	r.closeTcpF = func() error {
		return mwssServer.Close()
	}
	r.l.Infof("Start MWSS relay Server: %s", r.Name)
	return mwssServer.ListenAndServe()
}
