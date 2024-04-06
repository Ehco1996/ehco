package relay

import (
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/transporter"
)

type Relay struct {
	Name string // unique name for all relay

	TP transporter.RelayTransporter

	cfg *conf.Config
	l   *zap.SugaredLogger
}

func NewRelay(cfg *conf.Config, connMgr cmgr.Cmgr) (*Relay, error) {
	tp, err := transporter.NewRelayTransporter(cfg, connMgr)
	if err != nil {
		return nil, err

	}
	r := &Relay{
		TP:   tp,
		cfg:  cfg,
		Name: cfg.Label,
		l:    zap.S().Named("relay"),
	}

	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errCh := make(chan error)
	go func() {
		r.l.Infof("Start TCP Relay Server:%s", r.cfg.DefaultLabel())
		errCh <- r.TP.ListenAndServe()
	}()
	return <-errCh
}

func (r *Relay) Close() {
	r.l.Infof("Close relay label: %s", r.Name)
	if err := r.TP.Close(); err != nil {
		r.l.Errorf(err.Error())
	}
}
