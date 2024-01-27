package web

import (
	"fmt"
	"net/http"

	"github.com/Ehco1996/ehco/internal/constant"
	"go.uber.org/zap"
)

func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}

func welcome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, constant.WelcomeHTML)
}

func writerBadRequestMsg(w http.ResponseWriter, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(msg))
}

func (s *Server) HandleClashProxyProvider(w http.ResponseWriter, r *http.Request) {
	subName := r.URL.Query().Get("sub_name")
	if subName == "" {
		msg := "sub_name is empty"
		writerBadRequestMsg(w, msg)
		return
	}
	if s.relayServerReloader != nil {
		s.relayServerReloader.TriggerReload()
	} else {
		s.l.Debugf("relayServerReloader is nil this should not happen")
	}

	clashSubList, err := s.cfg.GetClashSubList()
	if err != nil {
		writerBadRequestMsg(w, err.Error())
		return
	}
	for _, clashSub := range clashSubList {
		if clashSub.Name == subName {
			clashCfgBuf, err := clashSub.ToClashConfigYaml()
			if err != nil {
				writerBadRequestMsg(w, err.Error())
				return
			}

			_, err = w.Write(clashCfgBuf)
			if err != nil {
				s.l.Errorf("write response meet err=%v", err)
				return
			}
			return
		}
	}
	msg := fmt.Sprintf("sub_name=%s not found", subName)
	writerBadRequestMsg(w, msg)
}

func (s *Server) HandleReload(w http.ResponseWriter, r *http.Request) {
	if s.relayServerReloader == nil {
		writerBadRequestMsg(w, "reload not support")
		return
	}

	s.relayServerReloader.TriggerReload()
	_, err := w.Write([]byte("reload success"))
	if err != nil {
		s.l.Errorf("write response meet err=%v", err)
		writerBadRequestMsg(w, err.Error())
		return
	}
}
