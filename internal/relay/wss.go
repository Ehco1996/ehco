package relay

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gorilla/websocket"
)

type WsConn struct {
	conn *websocket.Conn
	rb   []byte
}

func (c *WsConn) Read(b []byte) (n int, err error) {
	if len(c.rb) == 0 {
		_, c.rb, err = c.conn.ReadMessage()
	}
	n = copy(b, c.rb)
	c.rb = c.rb[n:]
	return
}

func (c *WsConn) Write(b []byte) (n int, err error) {
	err = c.conn.WriteMessage(websocket.BinaryMessage, b)
	n = len(b)
	return
}

func (c *WsConn) Close() error {
	return c.conn.Close()
}

func (c *WsConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *WsConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *WsConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}
func (c *WsConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *WsConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func newWsConn(conn *websocket.Conn) *WsConn {
	wsc := &WsConn{conn: conn}
	return wsc
}

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
	drc := NewDeadLinerConn(rc, TcpDeadline)
	defer drc.Close()
	Logger.Infof("handleWssToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	transport(drc, wsc)
}

func (relay *Relay) handleTcpOverWss(c *net.TCPConn) error {
	dc := NewDeadLinerConn(c, TcpDeadline)
	defer dc.Close()

	d := ws.Dialer{TLSConfig: DefaultTLSConfig}
	rc, _, _, err := d.Dial(context.TODO(), relay.RemoteTCPAddr+"/tcp/")
	if err != nil {
		return err
	}

	wsc := NewDeadLinerConn(rc, WsDeadline)
	defer wsc.Close()
	transport(dc, wsc)
	return nil
}

func (relay *Relay) handleWssToUdp(w http.ResponseWriter, r *http.Request) {
	Logger.Info("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWss(addr string, ubc *udpBufferCh) {
	Logger.Info("not support relay udp over ws currently")
}
