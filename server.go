package ehco

import (
	"encoding/json"
	"github.com/soheilhy/cmux"
	"log"
	"net"
	"net/http"
	"strings"
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

func serveRelay(l net.Listener) {
	log.Println("[INFO] start relay server")
	r, _ := NewRelay("0.0.0.0:1234", "0.0.0.0:9002")
	for {
		conn, err := l.Accept()
		log.Println("coon", conn.RemoteAddr().String())
		if err != nil {
			panic(err)
		}
		go r.HandleConn(conn)
	}

}

func StartMuxServer() error {
	l, err := net.Listen("tcp", "0.0.0.0:1234")
	if err != nil {
		log.Panic(err)
	}
	m := cmux.New(l)

	httpListener := m.Match(cmux.HTTP1())
	anyListener := m.Match(cmux.Any())
	m.Match(cmux.TLS())
	// register relay
	go serveRelay(anyListener)

	//register http
	go serveHTTP(httpListener)

	// start server
	if err := m.Serve(); !strings.Contains(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}
