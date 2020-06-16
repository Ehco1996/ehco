package relay

import (
	"fmt"
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
		return
	}
	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		log.Printf("dial error: %s", err)
		return
	}
	log.Printf("handleWsToTcp from:%s to:%s", c.RemoteAddr(), rc.RemoteAddr())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		buf := inboundBufferPool.Get().([]byte)
		defer func() {
			// c.Close()
			// rc.Close()
			relay.fastTimeout(c)
			relay.fastTimeout(rc)
			inboundBufferPool.Put(buf)
			wg.Done()
		}()
		for {
			relay.keepAliveAndSetNextTimeout(rc)
			relay.keepAliveAndSetNextTimeout(c)
			var n int
			if n, err = rc.Read(buf[:]); err != nil {
				break
			}
			c.WriteMessage(websocket.BinaryMessage, buf[0:n])
		}
	}()

	for {
		relay.keepAliveAndSetNextTimeout(rc)
		relay.keepAliveAndSetNextTimeout(c)
		var message []byte
		if _, message, err = c.ReadMessage(); err != nil {
			break
		}
		rc.Write(message)
	}
	relay.fastTimeout(c)
	relay.fastTimeout(rc)
	wg.Wait()
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	d := websocket.Dialer{TLSClientConfig: DefaultTLSConfig}
	rc, _, err := d.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// defer rc.Close()
		relay.fastTimeout(c)
		relay.fastTimeout(rc)
		defer wg.Done()
		for {
			relay.keepAliveAndSetNextTimeout(rc)
			relay.keepAliveAndSetNextTimeout(c)
			var message []byte
			if _, message, err = rc.ReadMessage(); err != nil {
				break
			}
			c.Write(message)
		}
	}()

	buf := inboundBufferPool.Get().([]byte)
	for {
		relay.keepAliveAndSetNextTimeout(rc)
		relay.keepAliveAndSetNextTimeout(c)
		var n int
		if n, err = c.Read(buf[:]); err != nil {
			break
		}
		rc.WriteMessage(websocket.BinaryMessage, buf[0:n])
	}
	inboundBufferPool.Put(buf)
	relay.fastTimeout(c)
	relay.fastTimeout(rc)
	wg.Wait()
	return err
}

func (relay *Relay) handleWsToUdp(w http.ResponseWriter, r *http.Request) {
	log.Println("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWs(addr string, ubc *udpBufferCh) {
	log.Println("not support relay udp over ws currently")
}
