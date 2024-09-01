package relay

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/config"
	"go.uber.org/zap"
)

type Server struct {
	relayM *sync.Map
	cfg    *config.Config
	l      *zap.SugaredLogger

	errCH    chan error    // once error happen, server will exit
	reloadCH chan struct{} // reload config

	Cmgr cmgr.Cmgr
}

func NewServer(cfg *config.Config) (*Server, error) {
	l := zap.S().Named("relay-server")
	cmgrCfg := &cmgr.Config{
		SyncURL:      cfg.RelaySyncURL,
		SyncInterval: cfg.RelaySyncInterval,
		MetricsURL:   cfg.GetMetricURL(),
	}
	cmgrCfg.Adjust()
	cmgr, err := cmgr.NewCmgr(cmgrCfg)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		l:        l,
		relayM:   &sync.Map{},
		errCH:    make(chan error, 1),
		reloadCH: make(chan struct{}, 1),
		Cmgr:     cmgr,
	}
	return s, nil
}

func (s *Server) startOneRelay(ctx context.Context, r *Relay) {
	s.relayM.Store(r.UniqueID(), r)
	// mute closed network error for tcp server and mute http.ErrServerClosed for http server when config reload
	if err := r.ListenAndServe(ctx); err != nil &&
		!errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
		s.l.Errorf("start relay %s meet error: %s", r.UniqueID(), err)
		s.errCH <- err
	}
}

func (s *Server) stopOneRelay(r *Relay) {
	_ = r.Stop()
	s.relayM.Delete(r.UniqueID())
}

func (s *Server) Start(ctx context.Context) error {
	// init and relay servers
	for idx := range s.cfg.RelayConfigs {
		r, err := NewRelay(s.cfg.RelayConfigs[idx], s.Cmgr)
		if err != nil {
			return err
		}
		go s.startOneRelay(ctx, r)
	}

	if s.cfg.PATH != "" && (s.cfg.ReloadInterval > 0) {
		s.l.Infof("Start to watch relay config %s ", s.cfg.PATH)
		go s.WatchAndReload(ctx)
	}

	// start Cmgr
	go s.Cmgr.Start(ctx, s.errCH)

	select {
	case err := <-s.errCH:
		s.l.Errorf("meet error: %s exit now.", err)
		return err
	case <-ctx.Done():
		s.l.Info("ctx cancelled start to stop all relay servers")
		return s.Stop()
	}
}

func (s *Server) Stop() error {
	var err error
	s.relayM.Range(func(key, value interface{}) bool {
		r := value.(*Relay)
		if e := r.Stop(); e != nil {
			err = errors.Join(err, e)
		}
		return true
	})
	return err
}

func (s *Server) TriggerReload() {
	s.reloadCH <- struct{}{}
}

func (s *Server) WatchAndReload(ctx context.Context) {
	go s.TriggerReloadBySignal(ctx)
	go s.triggerReloadByTicker(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reloadCH:
			if err := s.Reload(false); err != nil {
				s.l.Errorf("auto reloading relay conf meet error: %s will retry in next loop", err)
			}
		}
	}
}

func (s *Server) triggerReloadByTicker(ctx context.Context) {
	if s.cfg.ReloadInterval > 0 {
		ticker := time.NewTicker(time.Second * time.Duration(s.cfg.ReloadInterval))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.l.Warn("Trigger Reloading Relay Conf By ticker! ")
				s.TriggerReload()
			}
		}
	}
}

func (s *Server) TriggerReloadBySignal(ctx context.Context) {
	// listen syscall.SIGHUP to trigger reload
	sigHubCH := make(chan os.Signal, 1)
	signal.Notify(sigHubCH, syscall.SIGHUP)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sigHubCH:
			s.l.Warn("Trigger Reloading Relay Conf By HUP Signal! ")
			s.TriggerReload()
		}
	}
}
