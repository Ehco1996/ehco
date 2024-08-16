package relay

import (
	"context"

	"github.com/Ehco1996/ehco/internal/glue"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"go.uber.org/zap"
)

// make sure Server implements the reloader.Reloader interface
var _ glue.Reloader = (*Server)(nil)

func (s *Server) Reload(force bool) error {
	// k:name v: *Config
	oldRelayCfgM := make(map[string]*conf.Config)
	for _, v := range s.cfg.RelayConfigs {
		oldRelayCfgM[v.Label] = v.Clone()
	}
	allRelayLabelList := make([]string, 0)

	// NOTE: this is for reuse cached clash sub, because clash sub to relay config will change port every time when call
	if err := s.cfg.LoadConfig(force); err != nil {
		s.l.Error("load new cfg meet error", zap.Error(err))
		return err
	}

	// find all new relay label
	for _, newCfg := range s.cfg.RelayConfigs {
		// start bread new relay that not in old relayM
		allRelayLabelList = append(allRelayLabelList, newCfg.Label)
	}
	// closed relay not in all relay list
	s.relayM.Range(func(key, value interface{}) bool {
		oldLabel := key.(string)
		if !inArray(oldLabel, allRelayLabelList) {
			v, _ := s.relayM.Load(oldLabel)
			oldR := v.(*Relay)
			s.stopOneRelay(oldR)
		}
		return true
	})

	for _, newCfg := range s.cfg.RelayConfigs {
		// start bread new relay that not in old relayM
		if old, ok := s.relayM.Load(newCfg.Label); !ok {
			s.l.Infof("start new relay name=%s", newCfg.Label)
			r, err := NewRelay(newCfg, s.Cmgr)
			if err != nil {
				s.l.Error("new relay meet error", zap.Error(err))
				continue
			}
			go s.startOneRelay(context.TODO(), r)
		} else {
			// when label not change, check if config changed
			oldCfg, ok := oldRelayCfgM[newCfg.Label]
			if !ok {
				continue
				/// should not happen
			}
			// stop old and start new relay when config changed
			if oldCfg.Different(newCfg) {
				oldR := old.(*Relay)
				s.l.Infof("relay config changed, stop old and start new relay name=%s", newCfg.Label)
				s.stopOneRelay(oldR)
				r, err := NewRelay(newCfg, s.Cmgr)
				if err != nil {
					s.l.Error("new relay meet error", zap.Error(err))
					continue
				}
				go s.startOneRelay(context.TODO(), r)
			}
		}
	}

	return nil
}
