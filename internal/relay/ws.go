package relay

import (
	"fmt"
	"log"
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

func (relay *Relay) RunLocalWsServer() error {
	http.HandleFunc("/tcp/", relay.handleWsToTcp)
	http.HandleFunc("/udp/", relay.handleWsToUdp)
	// fake
	http.HandleFunc("/", index)
	// TODO 加上server的keepalive和read time out
	return http.ListenAndServeTLS(relay.LocalTCPAddr.String(), CertFileName, KeyFileName, nil)
}

func index(w http.ResponseWriter, r *http.Request) {
	log.Printf("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func (relay *Relay) handleWsToTcp(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wsc := newWsConn(conn)
	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		log.Printf("dial error: %s", err)
		return
	}
	log.Printf("handleWsToTcp from:%s to:%s", wsc.RemoteAddr(), rc.RemoteAddr())
	transport(rc, wsc)
	rc.Close()
	wsc.Close()
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	d := websocket.Dialer{TLSClientConfig: DefaultTLSConfig}
	conn, resp, err := d.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	wsc := newWsConn(conn)
	transport(c, wsc)
	c.Close()
	wsc.Close()
	return nil
}

func (relay *Relay) handleWsToUdp(w http.ResponseWriter, r *http.Request) {
	log.Println("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWs(addr string, ubc *udpBufferCh) {
	log.Println("not support relay udp over ws currently")
}
