package relay

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// use default options
var upgrader = websocket.Upgrader{}

func (relay *Relay) RunLocalWsServer() error {
	http.HandleFunc("/tcp/", relay.handleWsToTcp)
	http.HandleFunc("/udp/", relay.handleWsToUdp)
	// fake
	http.HandleFunc("/", index)
	return http.ListenAndServeTLS(relay.LocalTCPAddr.String(), CertFileName, KeyFileName, nil)
}

func index(w http.ResponseWriter, r *http.Request) {
	log.Printf("index call from %s", r.RemoteAddr)
	fmt.Fprintf(w, "access from %s \n", r.RemoteAddr)
}

func (relay *Relay) handleWsToTcp(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer c.Close()

	rc, _ := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		log.Println("dial:", err)
		return
	}
	defer rc.Close()
	log.Printf("handleWsToTcp from:%s to:%s", c.RemoteAddr(), rc.RemoteAddr())

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		buf := inboundBufferPool.Get().([]byte)
		for {
			if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
				break
			}
			var n int
			if n, err = rc.Read(buf[:]); err != nil {
				if err != io.EOF {
					log.Println("read error", err, "inin")
				}
				break
			}

			if err := c.SetWriteDeadline(time.Now().Add(WsDeadline)); err != nil {
				break
			}
			c.WriteMessage(websocket.BinaryMessage, buf[0:n])
		}
		inboundBufferPool.Put(buf)
		wg.Done()
	}()

	for {
		if err := c.SetReadDeadline(time.Now().Add(WsDeadline)); err != nil {
			break
		}
		var message []byte
		if _, message, err = c.ReadMessage(); err != nil {
			log.Println("read error:", err)
			break
		}

		if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
			break
		}
		rc.Write(message)
	}
	wg.Wait()
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	d := websocket.Dialer{TLSClientConfig: DefaultTLSConfig}
	rc, _, err := d.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	defer rc.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		for {
			if err := rc.SetReadDeadline(time.Now().Add(WsDeadline)); err != nil {
				break
			}
			var message []byte
			if _, message, err = rc.ReadMessage(); err != nil {
				if err != io.EOF {
					log.Println("read error", err)
				}
				break
			}

			if err := relay.keepAliveAndSetNextTimeout(c); err != nil {
				break
			}
			if _, err := c.Write(message); err != nil {
				log.Println("write error", err)
				break
			}
		}
		wg.Done()
	}()

	buf := inboundBufferPool.Get().([]byte)
	for {
		if err := relay.keepAliveAndSetNextTimeout(c); err != nil {
			break
		}
		var n int
		if n, err = c.Read(buf[:]); err != nil {
			if err != io.EOF {
				log.Println("read error", err)
			}
			break
		}

		if err := rc.SetWriteDeadline(time.Now().Add(WsDeadline)); err != nil {
			break
		}
		if err := rc.WriteMessage(websocket.BinaryMessage, buf[0:n]); err != nil {
			log.Println("write error", err)
			break
		}

	}
	inboundBufferPool.Put(buf)
	wg.Wait()
	return err
}

func (relay *Relay) handleWsToUdp(w http.ResponseWriter, r *http.Request) {
	log.Println("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWs(addr string, ubc *udpBufferCh) {
	log.Println("not support relay udp over ws currently")
}
