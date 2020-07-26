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

func (r *Relay) RunLocalWSServer() error {

	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/ws/tcp/", r.handleWsToTcp)
	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()
	return server.Serve(ln)
}

func (r *Relay) handleWsToTcp(w http.ResponseWriter, req *http.Request) {
	wsc, _, _, err := ws.UpgradeHTTP(req, w)
	if err != nil {
		return
	}
	defer wsc.Close()

	rc, err := net.Dial("tcp", r.RemoteTCPAddr)
	if err != nil {
		Logger.Infof("dial error: %s", err)
		return
	}
	defer rc.Close()
	Logger.Infof("handleWsToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	transport(rc, wsc)
}

func (r *Relay) handleTcpOverWs(c *net.TCPConn) error {
	defer c.Close()

	addr, node := r.PickTcpRemote()
	if node != nil {
		defer r.LBRemotes.DeferPick(node)
	}
	addr += "/ws/tcp/"

	wsc, _, _, err := ws.Dial(context.TODO(), addr)
	if err != nil {
		return err
	}
	defer wsc.Close()
	transport(c, wsc)
	return nil
}
