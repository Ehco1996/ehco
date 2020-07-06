package relay

import (
	"context"
	"github.com/gobwas/ws"
	"net"
	"net/http"
	"time"
)

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
	c, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	wsc := NewDeadLinerConn(c, WsDeadline)
	defer wsc.Close()

	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		Logger.Infof("dial error: %s", err)
		return
	}
	defer rc.Close()

	Logger.Infof("handleWsToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())

	if err := rc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		Logger.Infof("set deadline error: %s", err)
		return
	}
	transport(rc, wsc)
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	defer c.Close()
	rc, _, _, err := ws.Dial(context.TODO(), relay.RemoteTCPAddr+"/ws/tcp/")
	if err != nil {
		return err
	}
	wsc := NewDeadLinerConn(rc, WsDeadline)
	defer wsc.Close()

	if err := c.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		return err
	}
	transport(c, wsc)
	return nil
}
