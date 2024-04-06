package relay

import (
	"net"

	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/transporter"
)

type Relay struct {
	Name          string // unique name for all relay
	TransportType string
	ListenType    string

	TP transporter.RelayTransporter

	LocalTCPAddr *net.TCPAddr
	closeTcpF    func() error

	cfg *conf.Config
	l   *zap.SugaredLogger
}

func NewRelay(cfg *conf.Config, connMgr cmgr.Cmgr) (*Relay, error) {
	localTCPAddr, err := net.ResolveTCPAddr("tcp", cfg.Listen)
	if err != nil {
		return nil, err
	}

	r := &Relay{
		cfg: cfg,
		l:   zap.S().Named("relay"),

		Name:          cfg.Label,
		LocalTCPAddr:  localTCPAddr,
		ListenType:    cfg.ListenType,
		TransportType: cfg.TransportType,
		TP:            transporter.NewRelayTransporter(cfg, connMgr),
	}

	return r, nil
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
	return <-errCh
}

func (r *Relay) Close() {
	r.l.Infof("Close relay label: %s", r.Name)
	if r.closeTcpF != nil {
		err := r.closeTcpF()
		if err != nil {
			r.l.Errorf(err.Error())
		}
	}
}

func (r *Relay) RunLocalTCPServer() error {
	rawServer, err := transporter.NewRawServer(r.LocalTCPAddr.String(), r.TP)
	if err != nil {
		return err
	}
	r.closeTcpF = func() error {
		return rawServer.Close()
	}
	r.l.Infof("Start TCP relay Server: %s", r.Name)
	return rawServer.ListenAndServe()
}

func (r *Relay) RunLocalMTCPServer() error {
	tp := r.TP.(*transporter.RawClient)
	mTCPServer := transporter.NewMTCPServer(r.LocalTCPAddr.String(), tp, r.l.Named("MTCPServer"))
	r.closeTcpF = func() error {
		return mTCPServer.Close()
	}
	r.l.Infof("Start MTCP relay Server: %s", r.Name)
	return mTCPServer.ListenAndServe()
}

func (r *Relay) RunLocalWSServer() error {
	tp := r.TP.(*transporter.RawClient)
	wsServer := transporter.NewWSServer(r.LocalTCPAddr.String(), tp, r.l.Named("WSServer"))
	r.closeTcpF = func() error {
		return wsServer.Close()
	}
	r.l.Infof("Start WS relay Server: %s", r.Name)
	return wsServer.ListenAndServe()
}

func (r *Relay) RunLocalWSSServer() error {
	tp := r.TP.(*transporter.RawClient)
	wssServer := transporter.NewWSSServer(r.LocalTCPAddr.String(), tp, r.l.Named("WSSServer"))
	r.closeTcpF = func() error {
		return wssServer.Close()
	}
	r.l.Infof("Start WSS relay Server: %s", r.Name)
	return wssServer.ListenAndServe()
}

func (r *Relay) RunLocalMWSSServer() error {
	tp := r.TP.(*transporter.RawClient)
	mwssServer := transporter.NewMWSSServer(r.LocalTCPAddr.String(), tp, r.l.Named("MWSSServer"))
	r.closeTcpF = func() error {
		return mwssServer.Close()
	}
	r.l.Infof("Start MWSS relay Server: %s", r.Name)
	return mwssServer.ListenAndServe()
}
