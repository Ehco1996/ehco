package web

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

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

func registerMetrics(pingHosts []string) {
	// traffic
	prometheus.MustRegister(CurTCPNum)
	prometheus.MustRegister(CurUDPNum)
	prometheus.MustRegister(NetWorkTransmitBytes)

	//ping
	pg := NewPingGroup(pingHosts)
	pg.Run()
	prometheus.MustRegister(PingResponseDurationSeconds)
	prometheus.MustRegister(NewSmokepingCollector(pg, *PingResponseDurationSeconds))
}

func StartWebServer(port string, pingHosts []string) {
	addr := "0.0.0.0:" + port
	logger.Infof("[prom] Start Web Server at http://%s/", addr)
	r := mux.NewRouter()
	AttachProfiler(r)
	registerMetrics(pingHosts)
	r.Handle("/metrics/", promhttp.Handler())
	r.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, constant.IndexHTMLTMPL)
		})
	logger.Fatal(http.ListenAndServe(addr, r))
}
