package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"go.uber.org/zap"
)

func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}

func writerBadRequestMsg(w http.ResponseWriter, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(msg))
}

func (s *Server) welcome(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("").Parse(constant.WelcomeHTML))
	data := struct {
		Version     string
		GitBranch   string
		GitRevision string
		BuildTime   string
		SubConfigs  []*config.SubConfig
	}{
		Version:     constant.Version,
		GitBranch:   constant.GitBranch,
		GitRevision: constant.GitRevision,
		BuildTime:   constant.BuildTime,
		SubConfigs:  s.cfg.SubConfigs,
	}
	if err := tmpl.Execute(w, data); err != nil {
		writerBadRequestMsg(w, err.Error())
		return
	}
}

func (s *Server) HandleClashProxyProvider(w http.ResponseWriter, r *http.Request) {
	subName := r.URL.Query().Get("sub_name")
	if subName == "" {
		msg := "sub_name is empty"
		writerBadRequestMsg(w, msg)
		return
	}
	grouped := r.URL.Query().Get("grouped")
	if grouped == "true" {
		handleClashProxyProvider(s, w, r, subName, true)
	} else {
		handleClashProxyProvider(s, w, r, subName, false)
	}
}

func handleClashProxyProvider(s *Server, w http.ResponseWriter, r *http.Request, subName string, grouped bool) {
	if s.relayServerReloader != nil {
		if err := s.relayServerReloader.Reload(); err != nil {
			writerBadRequestMsg(w, err.Error())
			return
		}
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
			var clashCfgBuf []byte
			var err error
			if grouped {
				clashCfgBuf, err = clashSub.ToGroupedClashConfigYaml()
			} else {
				clashCfgBuf, err = clashSub.ToClashConfigYaml()
			}
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

	s.relayServerReloader.Reload()
	_, err := w.Write([]byte("reload success"))
	if err != nil {
		s.l.Errorf("write response meet err=%v", err)
		writerBadRequestMsg(w, err.Error())
		return
	}
}

func (s *Server) CurrentConfig(w http.ResponseWriter, r *http.Request) {
	// return json config
	ret, err := json.Marshal(s.cfg)
	if err != nil {
		writerBadRequestMsg(w, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(ret)
}
