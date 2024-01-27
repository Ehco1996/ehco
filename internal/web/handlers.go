package web

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/sub"
	"go.uber.org/zap"
)

func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}

func welcome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, constant.IndexHTMLTMPL)
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

	clashSubList, err := refreshClashProxyProvider(s.cfg)
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
			// todo refresh relay config and restart relay
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

func refreshClashProxyProvider(cfg *config.Config) ([]*sub.ClashSub, error) {
	clashSubList := make([]*sub.ClashSub, 0, len(cfg.SubConfigs))
	for _, subCfg := range cfg.SubConfigs {
		resp, err := http.Get(subCfg.URL)
		if err != nil {
			msg := fmt.Sprintf("http get sub config url=%s meet err=%v", subCfg.URL, err)
			return nil, fmt.Errorf(msg)

		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg := fmt.Sprintf("http get sub config url=%s meet status code=%d", subCfg.URL, resp.StatusCode)
			return nil, fmt.Errorf(msg)

		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			msg := fmt.Sprintf("read body meet err=%v", err)
			return nil, fmt.Errorf(msg)
		}
		clashSub, err := sub.NewClashSub(body, subCfg.Name)
		if err != nil {
			msg := fmt.Sprintf("NewClashSub meet err=%v", err)
			return nil, fmt.Errorf(msg)
		}
		clashSubList = append(clashSubList, clashSub)
	}
	return clashSubList, nil
}
