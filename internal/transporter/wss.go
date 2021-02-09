package transporter

// import (
// 	"context"
// 	"crypto/tls"
// 	"net"
// 	"net/http"
// 	"time"

// 	"github.com/gobwas/ws"
// )

// func (r *Relay) RunLocalWSSServer() error {
// 	mux := http.NewServeMux()
// 	mux.HandleFunc("/", index)
// 	mux.HandleFunc("/tcp/", r.handleWssToTcp)

// 	server := &http.Server{
// 		Addr:              r.LocalTCPAddr.String(),
// 		TLSConfig:         DefaultTLSConfig,
// 		ReadHeaderTimeout: 30 * time.Second,
// 		Handler:           mux,
// 	}
// 	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
// 	if err != nil {
// 		return err
// 	}
// 	defer ln.Close()
// 	return server.Serve(tls.NewListener(ln, server.TLSConfig))
// }

// func (r *Relay) handleWssToTcp(w http.ResponseWriter, req *http.Request) {
// 	wsc, _, _, err := ws.UpgradeHTTP(req, w)
// 	if err != nil {
// 		return
// 	}
// 	defer wsc.Close()

// 	rc, err := net.Dial("tcp", r.RemoteTCPAddr)
// 	if err != nil {
// 		logger.Logger.Infof("dial error: %s", err)
// 		return
// 	}
// 	defer rc.Close()
// 	logger.Logger.Infof("handleWssToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
// 	if err := transport(rc, wsc); err != nil {
// 		logger.Logger.Infof("handleWssToTcp err: %s", err.Error())
// 	}
// }

// func (r *Relay) handleTcpOverWss(c *net.TCPConn) error {
// 	defer c.Close()

// 	addr, node := r.PickTcpRemote()
// 	if node != nil {
// 		defer r.LBRemotes.DeferPick(node)
// 	}
// 	addr += "/tcp/"

// 	d := ws.Dialer{TLSConfig: DefaultTLSConfig}
// 	wsc, _, _, err := d.Dial(context.TODO(), addr)
// 	if err != nil {
// 		return err
// 	}
// 	defer wsc.Close()
// 	if err := transport(c, wsc); err != nil {
// 		logger.Logger.Infof("handleTcpOverWss err: %s", err.Error())
// 	}
// 	return nil
// }
