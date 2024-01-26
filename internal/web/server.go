package web

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/sub"
	"github.com/alecthomas/kingpin/v2"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"github.com/prometheus/node_exporter/collector"
	"go.uber.org/zap"
)

func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}

func Welcome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, constant.IndexHTMLTMPL)
}

func AttachProfiler(router *mux.Router) {
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Manually add support for paths linked to by index page at /debug/pprof/
	router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	router.Handle("/debug/pprof/block", pprof.Handler("block"))
}

func registerMetrics(cfg *config.Config) {
	// traffic
	prometheus.MustRegister(EhcoAlive)
	prometheus.MustRegister(CurConnectionCount)
	prometheus.MustRegister(NetWorkTransmitBytes)
	prometheus.MustRegister(HandShakeDuration)

	EhcoAlive.Set(EhcoAliveStateInit)

	// ping
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		prometheus.MustRegister(PingResponseDurationSeconds)
		prometheus.MustRegister(pg)
		go pg.Run()
	}
}

func registerNodeExporterMetrics(cfg *config.Config) error {
	level := &promlog.AllowedLevel{}
	// mute node_exporter logger
	if err := level.Set("error"); err != nil {
		return err
	}
	promlogConfig := &promlog.Config{Level: level}
	logger := promlog.New(promlogConfig)
	// see this https://github.com/prometheus/node_exporter/pull/2463
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		return err
	}
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return fmt.Errorf("couldn't create collector: %s", err)
	}
	// nc.Collectors = collectors
	prometheus.MustRegister(
		nc,
		version.NewCollector("node_exporter"),
	)
	return nil
}

func simpleTokenAuthMiddleware(token string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != token {
			msg := fmt.Sprintf("un auth request from %s", r.RemoteAddr)
			zap.S().Named("web").Error(msg)
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
			} else {
				panic(msg)
			}
			return
		}
		h.ServeHTTP(w, r)
	})
}

func StartWebServer(cfg *config.Config, clashSubList []*sub.ClashSub) error {
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.WebPort)
	zap.S().Named("web").Infof("Start Web Server at http://%s/", addr)

	clashSubHandler := func(w http.ResponseWriter, r *http.Request) {
		subName := r.URL.Query().Get("name")
		if subName == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, cs := range clashSubList {
			if cs.Name == subName {
				yamlBuf, err := cs.ToClashConfigYaml()
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Write(yamlBuf)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}

	r := mux.NewRouter()
	AttachProfiler(r)
	registerMetrics(cfg)
	if err := registerNodeExporterMetrics(cfg); err != nil {
		return err
	}
	r.Handle("/", http.HandlerFunc(Welcome))
	r.Handle("/metrics/", promhttp.Handler())
	r.Handle("/sub/", http.HandlerFunc(clashSubHandler))

	if cfg.WebToken != "" {
		return http.ListenAndServe(addr, simpleTokenAuthMiddleware(cfg.WebToken, r))
	} else {
		return http.ListenAndServe(addr, r)
	}
}
