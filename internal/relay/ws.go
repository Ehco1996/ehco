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
		for {
			relay.keepAliveAndSetNextTimeout(rc)
			var n int
			if n, err = rc.Read(buf[:]); err != nil {
				println("1", err.Error())
				break
			}
			relay.keepAliveAndSetNextTimeout(c)
			if err = c.WriteMessage(websocket.BinaryMessage, buf[0:n]); err != nil {
				println("2", err.Error())
				break
			}
		}
		println("called in")
		relay.fastTimeout(rc)
		relay.fastTimeout(c)
		println("called out")
		inboundBufferPool.Put(buf)
		wg.Done()
	}()

	for {
		relay.keepAliveAndSetNextTimeout(c)
		var message []byte
		if _, message, err = c.ReadMessage(); err != nil {
			println("3", err.Error())
			break
		}
		relay.keepAliveAndSetNextTimeout(rc)
		if _, err = rc.Write(message); err != nil {
			println("4", err.Error())
			break
		}
	}
	println("called in")
	relay.fastTimeout(c)
	relay.fastTimeout(rc)
	println("called out")
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
		for {
			relay.keepAliveAndSetNextTimeout(rc)
			var message []byte
			if _, message, err = rc.ReadMessage(); err != nil {
				println("5", err.Error())
				break
			}
			relay.keepAliveAndSetNextTimeout(c)
			if _, err = c.Write(message); err != nil {
				println("6", err.Error())
				break
			}
		}
		println("called in")
		relay.fastTimeout(rc)
		relay.fastTimeout(c)
		println("called out")
		wg.Done()
	}()

	buf := inboundBufferPool.Get().([]byte)
	for {
		relay.keepAliveAndSetNextTimeout(c)
		var n int
		if n, err = c.Read(buf[:]); err != nil {
			println("7", err.Error())
			break
		}
		relay.keepAliveAndSetNextTimeout(rc)
		if err = rc.WriteMessage(websocket.BinaryMessage, buf[0:n]); err != nil {
			println("8", err.Error())
			break
		}
	}
	inboundBufferPool.Put(buf)
	println("called in")
	relay.fastTimeout(c)
	relay.fastTimeout(rc)
	println("called out")
	wg.Wait()
	return err
}

func (relay *Relay) handleWsToUdp(w http.ResponseWriter, r *http.Request) {
	log.Println("not support relay udp over ws currently")
}

func (relay *Relay) handleUdpOverWs(addr string, ubc *udpBufferCh) {
	log.Println("not support relay udp over ws currently")
}
