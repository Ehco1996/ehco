package relay

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
)

var (
	TcpDeadline = 60 * time.Second
	UdpDeadline = 60 * time.Second
	DEBUG       = os.Getenv("EHCO_DEBUG")
)

const (
	Transport_RAW = "raw"
	Transport_WS  = "ws"
)

const (
	Listen_RAW = "raw"
	Listen_WS  = "ws"
)

type Relay struct {
	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	RemoteTCPAddr string
	RemoteUDPAddr string

	ListenType    string
	TransportType string

	TCPListener *net.TCPListener
	UDPConn     *net.UDPConn
}

func NewRelay(localAddr, listenType, remoteAddr, transportType string) (*Relay, error) {
	localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		return nil, err
	}
	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return nil, err
	}

	r := &Relay{
		LocalTCPAddr: localTCPAddr,
		LocalUDPAddr: localUDPAddr,

		RemoteTCPAddr: remoteAddr,
		RemoteUDPAddr: remoteAddr,

		ListenType:    listenType,
		TransportType: transportType,
	}
	if DEBUG != "" {
		go func() {
			log.Printf("[DEBUG] start pprof server at 0.0.0.0:6060")
			log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}
	return r, nil
}

func (relay *Relay) Shutdown() error {
	var err, err1 error
	if relay.TCPListener != nil {
		err = relay.TCPListener.Close()
	}
	if relay.UDPConn != nil {
		err1 = relay.UDPConn.Close()
	}
	if err != nil {
		return err
	}
	return err1
}

func (relay *Relay) ListenAndServe() error {
	errChan := make(chan error)
	log.Printf("start relay AT: %s Over: %s TO: %s Through %s",
		relay.LocalTCPAddr, relay.ListenType, relay.RemoteTCPAddr, relay.TransportType)

	if relay.ListenType == Listen_RAW {
		go func() {
			errChan <- relay.RunLocalTCPServer()
		}()
		go func() {
			errChan <- relay.RunLocalUDPServer()
		}()
	} else if relay.ListenType == Listen_WS {
		go func() {
			errChan <- relay.RunLocalWsServer()
		}()
	} else {
		log.Fatalf("unknown listen type: %s ", relay.ListenType)
	}
	return <-errChan
}

func (relay *Relay) RunLocalTCPServer() error {
	var err error
	relay.TCPListener, err = net.ListenTCP("tcp", relay.LocalTCPAddr)
	if err != nil {
		return err
	}
	defer relay.TCPListener.Close()
	for {
		c, err := relay.TCPListener.AcceptTCP()
		log.Printf("handle tcp con from: %s", c.RemoteAddr())
		if err != nil {
			return err
		}

		if relay.TransportType == Transport_WS {
			go func(c *net.TCPConn) {
				defer c.Close()
				if err := relay.handleTcpOverWs(c); err != nil {
					log.Printf("handleTcpOverWs err %s", err)
				}
			}(c)
		} else {
			go func(c *net.TCPConn) {
				defer c.Close()
				relay.keepAliveAndSetNextTimeout(c)
				if err := relay.handleTCPConn(c); err != nil {
					log.Printf("handleTCPConn err %s", err)
				}
			}(c)
		}
	}
}

func (relay *Relay) RunLocalUDPServer() error {
	var err error
	relay.UDPConn, err = net.ListenUDP("udp", relay.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer relay.UDPConn.Close()
	for {
		// NOTE  mtu一般是1500,设置为超过这个这个值就够用了
		buf := make([]byte, 1024*2)
		n, addr, err := relay.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		log.Printf("handle udp package from %s", addr)
		go func(addr *net.UDPAddr, b []byte) {
			if err := relay.handleUDP(addr, buf); err != nil {
				log.Printf("handleUDP err %s", err)
				return
			}
		}(addr, buf[0:n])
	}
}

func (relay *Relay) keepAliveAndSetNextTimeout(conn interface{}) error {
	switch c := conn.(type) {
	case *net.TCPConn:
		if err := c.SetDeadline(time.Now().Add(TcpDeadline)); err != nil {
			log.Printf("set tcp timout err %s", err)
			return err
		}
	case *net.UDPConn:
		if err := c.SetDeadline(time.Now().Add(UdpDeadline)); err != nil {
			log.Printf("set udp timout err %s", err)
			return err
		}
	default:
		return nil
	}
	return nil
}

func (relay *Relay) handleTCPConn(c *net.TCPConn) error {
	rc, err := net.Dial("tcp", relay.RemoteTCPAddr)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go doCopy(rc, c, inboundBufferPool, &wg)
	go doCopy(c, rc, outboundBufferPool, &wg)
	wg.Wait()
	return nil
}

func (relay *Relay) handleUDP(addr *net.UDPAddr, b []byte) error {
	rc, err := net.Dial("udp", relay.RemoteUDPAddr)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := relay.keepAliveAndSetNextTimeout(rc); err != nil {
		return err
	}

	if _, err := rc.Write(b); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	i, err := rc.Read(buf)
	if err != nil {
		return err
	}
	if _, err := relay.UDPConn.WriteToUDP(buf[0:i], addr); err != nil {
		return err
	}
	return nil
}
