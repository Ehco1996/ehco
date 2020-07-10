package relay

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

func (relay *Relay) RunLocalWSSServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/tcp/", relay.handleWssToTcp)
	mux.HandleFunc("/udp/", relay.handleWssToUdp)

	server := &http.Server{
		Addr:              relay.LocalTCPAddr.String(),
		TLSConfig:         DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
		Handler:           mux,
	}
	ln, err := net.Listen("tcp", relay.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()
	return server.Serve(tls.NewListener(ln, server.TLSConfig))
}

func (relay *Relay) handleWssToTcp(w http.ResponseWriter, r *http.Request) {
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
	Logger.Infof("handleWssToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	transport(rc, wsc)
}

func (relay *Relay) handleTcpOverWss(c *net.TCPConn) error {
	defer c.Close()

	d := ws.Dialer{TLSConfig: DefaultTLSConfig}
	wsc, _, _, err := d.Dial(context.TODO(), relay.RemoteTCPAddr+"/tcp/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	transport(c, wsc)
	return nil
}

func (relay *Relay) handleWssToUdp(w http.ResponseWriter, r *http.Request) {
	Logger.Info("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWss(addr string, ubc *udpBufferCh) {
	Logger.Info("not support relay udp over ws currently")
}
