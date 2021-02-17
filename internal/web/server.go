package web

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/gorilla/mux"
)

func Index(w http.ResponseWriter, r *http.Request) {
	logger.Logger.Infof("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func AttachProfiler(router *mux.Router) {
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
}

func StartWebServer(port string) {
	addr := "0.0.0.0:" + port
	logger.Logger.Infof("Start Web Server at http://%s/", addr)
	r := mux.NewRouter()
	AttachProfiler(r)
	r.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, constant.IndexHTMLTMPL)
		})
	logger.Logger.Fatal(http.ListenAndServe(addr, r))
}
