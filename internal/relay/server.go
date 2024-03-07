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
	}
	cmgrCfg.Adjust()
	s := &Server{
		cfg:      cfg,
		l:        l,
		relayM:   &sync.Map{},
		errCH:    make(chan error, 1),
		reloadCH: make(chan struct{}, 1),
		Cmgr:     cmgr.NewCmgr(cmgrCfg),
	}
	return s, nil
}

func (s *Server) startOneRelay(r *Relay) {
	s.relayM.Store(r.Name, r)
	// mute closed network error for tcp server and mute http.ErrServerClosed for http server when config reload
	if err := r.ListenAndServe(); err != nil &&
		!errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
		s.l.Errorf("start relay %s meet error: %s", r.Name, err)
		s.errCH <- err
	}
}

func (s *Server) stopOneRelay(r *Relay) {
	r.Close()
	s.relayM.Delete(r.Name)
}

func (s *Server) Start(ctx context.Context) error {
	// init and relay servers
	for idx := range s.cfg.RelayConfigs {
		r, err := NewRelay(s.cfg.RelayConfigs[idx], s.Cmgr)
		if err != nil {
			return err
		}
		go s.startOneRelay(r)
	}

	if s.cfg.PATH != "" && (s.cfg.ReloadInterval > 0 || len(s.cfg.SubConfigs) > 0) {
		s.l.Infof("Start to watch relay config %s ", s.cfg.PATH)
		go s.WatchAndReload(ctx)
	}

	// start Cmgr
	go s.Cmgr.Start(ctx, s.errCH)

	select {
	case err := <-s.errCH:
		s.l.Errorf("relay server meet error: %s exit now.", err)
		return err
	case <-ctx.Done():
		s.l.Info("ctx cancelled start to stop all relay servers")
		s.relayM.Range(func(key, value interface{}) bool {
			r := value.(*Relay)
			r.Close()
			return true
		})
		return nil
	}
}

func (s *Server) Stop() error {
	s.l.Info("relay server stop now")
	s.relayM.Range(func(key, value interface{}) bool {
		r := value.(*Relay)
		r.Close()
		return true
	})
	return nil
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
			if err := s.Reload(); err != nil {
				s.l.Errorf("Reloading Relay Conf meet error: %s ", err)
				s.errCH <- err
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
