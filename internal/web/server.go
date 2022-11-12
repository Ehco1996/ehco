package web

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"github.com/prometheus/node_exporter/collector"
	"go.uber.org/zap"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	l *zap.SugaredLogger
)

func MakeIndexF(logger *zap.SugaredLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
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

	//ping
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		prometheus.MustRegister(PingResponseDurationSeconds)
		prometheus.MustRegister(pg)
		go pg.Run()
	}
}

func registerNodeExporterMetrics(cfg *config.Config) error {
	level := &promlog.AllowedLevel{}
	if err := level.Set(cfg.LogLeveL); err != nil {
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
			l.Error(msg)
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

func StartWebServer(cfg *config.Config) error {
	// todo make this only doing once
	l = log.Logger.Named("web")
	// end todo

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.WebPort)
	l.Infof("Start Web Server at http://%s/", addr)

	r := mux.NewRouter()
	AttachProfiler(r)
	registerMetrics(cfg)
	if err := registerNodeExporterMetrics(cfg); err != nil {
		return err
	}
	r.Handle("/", http.HandlerFunc(Welcome))
	r.Handle("/metrics/", promhttp.Handler())

	if cfg.WebToken != "" {
		return http.ListenAndServe(addr, simpleTokenAuthMiddleware(cfg.WebToken, r))
	} else {
		return http.ListenAndServe(addr, r)
	}
}
