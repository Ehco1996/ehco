package relay

import (
	"github.com/Ehco1996/ehco/internal/reloader"
	"go.uber.org/zap"
)

// make sure Server implements the reloader.Reloader interface
var _ reloader.Reloader = (*Server)(nil)

func (s *Server) Reload() error {
	// load config on raw
	// NOTE: this is for reuse cached clash sub, because clash sub to relay config will change port every time when call
	if err := s.cfg.LoadConfig(); err != nil {
		s.l.Error("load new cfg meet error", zap.Error(err))
		return err
	}
	var allRelayAddrList []string
	for idx := range s.cfg.RelayConfigs {
		r, err := NewRelay(s.cfg.RelayConfigs[idx])
		if err != nil {
			s.l.Errorf("reload new relay failed err=%s", err.Error())
			return err
		}
		allRelayAddrList = append(allRelayAddrList, r.Name)
		// start bread new relay that not in old relayM
		if _, ok := s.relayM.Load(r.Name); !ok {
			s.l.Infof("start new relay name=%s", r.Name)
			go s.startOneRelay(r)
			continue
		}
	}

	// closed relay not in all relay list
	s.relayM.Range(func(key, value interface{}) bool {
		oldAddr := key.(string)
		if !inArray(oldAddr, allRelayAddrList) {
			v, _ := s.relayM.Load(oldAddr)
			oldR := v.(*Relay)
			s.stopOneRelay(oldR)
		}
		return true
	})
	return nil
}
