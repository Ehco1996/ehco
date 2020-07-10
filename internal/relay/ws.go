package relay

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

func index(w http.ResponseWriter, r *http.Request) {
	Logger.Infof("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func (relay *Relay) RunLocalWSServer() error {

	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/ws/tcp/", relay.handleWsToTcp)
	server := &http.Server{
		Addr:              relay.LocalTCPAddr.String(),
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	ln, err := net.Listen("tcp", relay.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()
	return server.Serve(ln)
}

func (relay *Relay) handleWsToTcp(w http.ResponseWriter, r *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	defer wsc.Close()

	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		Logger.Infof("dial error: %s", err)
		return
	}
	defer rc.Close()
	Logger.Infof("handleWsToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	transport(rc, wsc)
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	defer c.Close()
	wsc, _, _, err := ws.Dial(context.TODO(), relay.RemoteTCPAddr+"/ws/tcp/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	transport(c, wsc)
	return nil
}
