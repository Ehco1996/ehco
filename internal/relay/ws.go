package relay

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"

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
			var n int
			if n, err = rc.Read(buf[:]); err != nil {
				if err != io.EOF {
					log.Println("read error", err)
				}
				break
			}
			if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
				break
			}
			c.WriteMessage(websocket.BinaryMessage, buf[0:n])
			if err := relay.keepAliveAndSetNextTimeout(c); err != nil {
				break
			}
		}
		inboundBufferPool.Put(buf)
		wg.Done()
	}()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			break
		}
		rc.Write(message)
		if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
			break
		}
	}
	wg.Wait()
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	rc, _, err := websocket.DefaultDialer.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	defer rc.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		for {
			_, msg, err := rc.ReadMessage()
			if err != nil {
				if err != io.EOF {
					log.Println("read error", err)
				}
				break
			}
			if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
				break
			}
			if _, err := c.Write(msg); err != nil {
				log.Println("write error", err)
				break
			}
			if err := relay.keepAliveAndSetNextTimeout(c); err != nil {
				break
			}
		}
		wg.Done()
	}()

	buf := inboundBufferPool.Get().([]byte)
	for {
		n, err := c.Read(buf[:])
		if err != nil {
			if err != io.EOF {
				log.Println("read error", err)
			}
			break
		}
		if err := relay.keepAliveAndSetNextTimeout(c); err != nil {
			break
		}
		if err := rc.WriteMessage(websocket.BinaryMessage, buf[0:n]); err != nil {
			log.Println("write error", err)
			break
		}
		if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
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
