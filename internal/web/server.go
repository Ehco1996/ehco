package web

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Index(w http.ResponseWriter, r *http.Request) {
	logger.Infof("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
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
	prometheus.MustRegister(CurTCPNum)
	prometheus.MustRegister(CurUDPNum)
	prometheus.MustRegister(NetWorkTransmitBytes)

	//ping
	pg := NewPingGroup(cfg)
	prometheus.MustRegister(PingResponseDurationSeconds)
	prometheus.MustRegister(pg)
	go pg.Run()
}

func StartWebServer(port, token string, cfg *config.Config) {
	time.Sleep(time.Second)
	addr := "0.0.0.0:" + port
	logger.Infof("[web] Start Web Server at http://%s/", addr)
	r := mux.NewRouter()
	AttachProfiler(r)
	registerMetrics(cfg)
	r.Handle("/metrics/", SimpleTokenAuthMiddleware(token, promhttp.Handler()))
	r.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, constant.IndexHTMLTMPL)
		})
	logger.Fatal(http.ListenAndServe(addr, r))
}

func SimpleTokenAuthMiddleware(token string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != token {
			logger.Logger.Errorf("[web] unauthorsied from %s", r.RemoteAddr)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
