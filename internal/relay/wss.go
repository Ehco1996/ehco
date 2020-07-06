package relay

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

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

func index(w http.ResponseWriter, r *http.Request) {
	Logger.Infof("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func (relay *Relay) handleWssToTcp(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wsc := newWsConn(conn)
	defer wsc.Close()
	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		Logger.Infof("dial error: %s", err)
		return
	}
	defer rc.Close()
	Logger.Infof("handleWssToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	if err := wsc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		Logger.Infof("set deadline error: %s", err)
		return
	}
	if err := rc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		Logger.Infof("set deadline error: %s", err)
		return
	}
	transport(rc, wsc)
}

func (relay *Relay) handleTcpOverWss(c *net.TCPConn) error {
	defer c.Close()
	d := websocket.Dialer{TLSClientConfig: DefaultTLSConfig}
	conn, resp, err := d.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	wsc := newWsConn(conn)
	defer wsc.Close()
	if err := wsc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		return err
	}
	if err := c.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		return err
	}
	transport(c, wsc)
	return nil
}

func (relay *Relay) handleWssToUdp(w http.ResponseWriter, r *http.Request) {
	Logger.Info("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWss(addr string, ubc *udpBufferCh) {
	Logger.Info("not support relay udp over ws currently")
}
