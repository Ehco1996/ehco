package relay

import (
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/transporter"
)

type Relay struct {
	Name string // unique name for all relay

	ListenTP transporter.RelayTransporter

	cfg *conf.Config
	l   *zap.SugaredLogger
}

func NewRelay(cfg *conf.Config, connMgr cmgr.Cmgr) (*Relay, error) {
	base := transporter.NewBaseTransporter(cfg, connMgr)
	tp, err := transporter.NewRelayTransporter(cfg.ListenType, base)
	if err != nil {
		return nil, err
	}
	r := &Relay{
		ListenTP: tp,
		cfg:      cfg,
		Name:     cfg.Label,
		l:        zap.S().Named("relay"),
	}
	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errCh := make(chan error)
	go func() {
		r.l.Infof("Start TCP Relay Server:%s", r.cfg.DefaultLabel())
		errCh <- r.ListenTP.ListenAndServe()
	}()
	return <-errCh
}

func (r *Relay) Close() {
	r.l.Infof("Close TCP Relay Server:%s", r.cfg.DefaultLabel())
	if err := r.ListenTP.Close(); err != nil {
		r.l.Errorf(err.Error())
	}
}
