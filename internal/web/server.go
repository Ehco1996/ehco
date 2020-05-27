package web

import (
	"encoding/json"
	"github.com/soheilhy/cmux"
	"log"
	"net"
	"net/http"
)

type SimpleHTTPHandler struct{}

func (s *SimpleHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	renderConfig(w, r)
}

func renderConfig(w http.ResponseWriter, r *http.Request) {
	cfg := NewConfig("config.json")
	cfg.LoadConfig()
	res, err := json.Marshal(cfg.Configs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

func serveHTTP(l net.Listener) {
	simpleHttp := &http.Server{
		Handler: &SimpleHTTPHandler{},
	}
	log.Println("[INFO] start http mux server")
	if err := simpleHttp.Serve(l); err != cmux.ErrListenerClosed {
		panic(err)
	}
}
