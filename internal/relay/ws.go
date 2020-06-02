package relay

import (
	"log"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
)

// use default options
var upgrader = websocket.Upgrader{}

func (relay *Relay) RunLocalWsServer() error {
	http.HandleFunc("/tcp/", relay.handleWsToTcp)
	http.HandleFunc("/udp/", relay.handleWsToUdp)
	return http.ListenAndServe(relay.LocalTCPAddr.String(), nil)
}

func (relay *Relay) handleTcpOverWs(c *net.TCPConn) error {
	rc, _, err := websocket.DefaultDialer.Dial(relay.RemoteTCPAddr+"/tcp/", nil)
	if err != nil {
		return err
	}
	defer rc.Close()

	go func() {
		for {
			_, msg, err := rc.ReadMessage()
			if err != nil {
				return
			}
			if _, err := c.Write(msg); err != nil {
				return
			}
		}
	}()

	var buf [1024 * 2]byte
	for {
		relay.keepAliveAndSetNextTimeout(c)
		i, err := c.Read(buf[:])
		if err != nil {
			return err
		}
		if err := rc.WriteMessage(websocket.BinaryMessage, buf[0:i]); err != nil {
			return err
		}
	}

	return nil
}

func (relay *Relay) handleUdpOverWs(addr *net.UDPAddr, b []byte) error {
	// rc, _, err := websocket.DefaultDialer.Dial(relay.RemoteTCPAddr+"/udp/", nil)
	// if err != nil {
	// 	return err
	// }
	// defer rc.Close()

	// go func() {
	// 	for {
	// 		_, msg, err := rc.ReadMessage()
	// 		if err != nil {
	// 			return
	// 		}
	// 		if _, err := c.Write(msg); err != nil {
	// 			return
	// 		}
	// 	}
	// }()

	// var buf [1024 * 2]byte
	// for {
	// 	relay.keepAliveAndSetNextTimeout(c)
	// 	i, err := c.Read(buf[:])
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if err := rc.WriteMessage(websocket.BinaryMessage, buf[0:i]); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func (relay *Relay) handleWsToTcp(w http.ResponseWriter, r *http.Request) {

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	rc, _ := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		log.Print("dail:", err)
		return
	}
	defer rc.Close()

	go func() {
		var buf [1024 * 2]byte
		for {
			i, err := rc.Read(buf[:])
			if err != nil {
				log.Print(err)
				return
			}
			c.WriteMessage(websocket.BinaryMessage, buf[0:i])
		}
	}()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			break
		}
		rc.Write(message)
	}
}

func (relay *Relay) handleWsToUdp(w http.ResponseWriter, r *http.Request) {

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	rc, _ := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		log.Print("dail:", err)
		return
	}
	defer rc.Close()

	go func() {
		var buf [1024 * 2]byte
		for {
			i, err := rc.Read(buf[:])
			if err != nil {
				log.Print(err)
				return
			}
			c.WriteMessage(websocket.BinaryMessage, buf[0:i])
		}
	}()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			break
		}
		rc.Write(message)
	}
}
