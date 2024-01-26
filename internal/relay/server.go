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

	"github.com/Ehco1996/ehco/internal/config"
	"go.uber.org/zap"
)

func inArray(ele string, array []string) bool {
	for _, v := range array {
		if v == ele {
			return true
		}
	}
	return false
}

type Server struct {
	relayM *sync.Map
	cfg    *config.Config
	l      *zap.SugaredLogger

	errCH chan error // once error happen, server will exit
}

func NewServer(cfg *config.Config) (*Server, error) {
	l := zap.S().Named("relay-server")
	s := &Server{
		cfg:    cfg,
		l:      l,
		relayM: &sync.Map{},
		errCH:  make(chan error, 1),
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
		r, err := NewRelay(s.cfg.RelayConfigs[idx])
		if err != nil {
			return err
		}
		go s.startOneRelay(r)
	}

	if s.cfg.PATH != "" && s.cfg.ReloadInterval > 0 {
		s.l.Infof("Start to watch relay config %s ", s.cfg.PATH)
		go s.WatchAndReload(ctx)
	}

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

func (s *Server) Reload() error {
	// load new config
	if err := s.cfg.LoadConfig(); err != nil {
		s.l.Errorf("Reload conf meet error: %s ", err)
		return err
	}
	var newRelayAddrList []string
	for idx := range s.cfg.RelayConfigs {
		r, err := NewRelay(s.cfg.RelayConfigs[idx])
		if err != nil {
			s.l.Errorf("reload new relay failed err=%s", err.Error())
			return err
		}
		newRelayAddrList = append(newRelayAddrList, r.Name)
		// reload relay when name change
		if oldR, ok := s.relayM.Load(r.Name); ok {
			oldR := oldR.(*Relay)
			if oldR.Name != r.Name {
				s.l.Warnf("close old relay name=%s", oldR.Name)
				s.stopOneRelay(oldR)
				go s.startOneRelay(r)
			}
			continue // no need to reload
		}
		// start bread new relay that not in old relayM
		s.l.Infof("start new relay name=%s", r.Name)
		go s.startOneRelay(r)
	}

	// closed relay not in new config
	s.relayM.Range(func(key, value interface{}) bool {
		oldAddr := key.(string)
		if !inArray(oldAddr, newRelayAddrList) {
			v, _ := s.relayM.Load(oldAddr)
			oldR := v.(*Relay)
			s.stopOneRelay(oldR)
		}
		return true
	})
	return nil
}

func (s *Server) WatchAndReload(ctx context.Context) {
	reloadCH := make(chan struct{}, 1)
	// listen syscall.SIGHUP to trigger reload
	sigHubCH := make(chan os.Signal, 1)
	signal.Notify(sigHubCH, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigHubCH:
				s.l.Info("Now Reloading Relay Conf By HUP Signal! ")
				reloadCH <- struct{}{}
			}
		}
	}()

	// ticker to reload config
	ticker := time.NewTicker(time.Second * time.Duration(s.cfg.ReloadInterval))
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reloadCH <- struct{}{}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadCH:
			if err := s.Reload(); err != nil {
				s.l.Errorf("Reloading Relay Conf meet error: %s ", err)
				s.errCH <- err
			}
		}
	}
}
