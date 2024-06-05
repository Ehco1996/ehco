package relay

import (
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/transporter"
)

type Relay struct {
	cfg *conf.Config
	l   *zap.SugaredLogger

	relayServer transporter.RelayServer
}

func (r *Relay) UniqueID() string {
	return r.cfg.Label
}

func NewRelay(cfg *conf.Config, cmgr cmgr.Cmgr) (*Relay, error) {
	s, err := transporter.NewRelayServer(cfg, cmgr)
	if err != nil {
		return nil, err
	}

	r := &Relay{
		relayServer: s,
		cfg:         cfg,
		l:           zap.S().Named("relay"),
	}
	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errCh := make(chan error)
	go func() {
		r.l.Infof("Start TCP Relay Server:%s", r.cfg.DefaultLabel())
		errCh <- r.relayServer.ListenAndServe()
	}()
	return <-errCh
}

func (r *Relay) Close() {
	r.l.Infof("Close TCP Relay Server:%s", r.cfg.DefaultLabel())
	if err := r.relayServer.Close(); err != nil {
		r.l.Errorf(err.Error())
	}
}
