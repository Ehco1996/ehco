package relay

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
)

var (
	TcpDeadline = 60 * time.Second
	UdpDeadline = 6 * time.Second
	DEBUG       = os.Getenv("EHCO_DEBUG")
)

const (
	Listen_RAW = "raw"
	Listen_WS  = "ws"

	Transport_RAW = "raw"
	Transport_WS  = "ws"
)

type Relay struct {
	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	RemoteTCPAddr string
	RemoteUDPAddr string

	ListenType    string
	TransportType string

	// may not init
	TCPListener *net.TCPListener
	UDPConn     *net.UDPConn

	udpCache map[string]*udpBufferCh
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

		udpCache: make(map[string](*udpBufferCh)),
	}
	if DEBUG != "" {
		go func() {
			log.Printf("[DEBUG] start pprof server at http://127.0.0.1:6060/debug/pprof/")
			log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}
	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errChan := make(chan error)
	log.Printf("start relay AT: %s Over: %s TO: %s Through %s",
		r.LocalTCPAddr, r.ListenType, r.RemoteTCPAddr, r.TransportType)

	if r.ListenType == Listen_RAW {
		go func() {
			errChan <- r.RunLocalTCPServer()
		}()
		go func() {
			errChan <- r.RunLocalUDPServer()
		}()
	} else if r.ListenType == Listen_WS {
		go func() {
			errChan <- r.RunLocalWsServer()
		}()
	} else {
		log.Fatalf("unknown listen type: %s ", r.ListenType)
	}
	return <-errChan
}

func (r *Relay) RunLocalTCPServer() error {
	var err error
	r.TCPListener, err = net.ListenTCP("tcp", r.LocalTCPAddr)
	if err != nil {
		return err
	}
	defer r.TCPListener.Close()
	for {
		c, err := r.TCPListener.AcceptTCP()
		log.Printf("handle tcp con from: %s over: %s", c.RemoteAddr(), r.TransportType)
		if err != nil {
			return err
		}

		switch r.TransportType {
		case Transport_WS:
			go func(c *net.TCPConn) {
				defer c.Close()
				if err := r.handleTcpOverWs(c); err != nil {
					log.Printf("handleTcpOverWs err %s", err)
				}
			}(c)
		case Transport_RAW:
			go func(c *net.TCPConn) {
				defer c.Close()
				r.keepAliveAndSetNextTimeout(c)
				if err := r.handleTCPConn(c); err != nil {
					log.Printf("handleTCPConn err %s", err)
				}
			}(c)
		}
	}
}

func (r *Relay) RunLocalUDPServer() error {
	var err error
	r.UDPConn, err = net.ListenUDP("udp", r.LocalUDPAddr)
	if err != nil {
		return err
	}
	defer r.UDPConn.Close()

	for {
		buf := inboundBufferPool.Get().([]byte)
		n, addr, err := r.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		log.Printf("handle udp package from %s over: %s len: %d", addr, r.ListenType, n)
		ubc, err := r.getOrCreateUbc(addr)
		if err != nil {
			return err
		}
		ubc.Ch <- buf[0:n]
		if !ubc.Handled {
			ubc.Handled = true
			go r.handleOneUDPConn(addr.String(), ubc)
		}
	}
}

func (r *Relay) keepAliveAndSetNextTimeout(conn interface{}) error {
	switch c := conn.(type) {
	case *net.TCPConn:
		if err := c.SetDeadline(time.Now().Add(TcpDeadline)); err != nil {
			return err
		}
	case *net.UDPConn:
		if err := c.SetDeadline(time.Now().Add(UdpDeadline)); err != nil {
			return err
		}
	default:
		return nil
	}
	return nil
}

// NOTE not thread safe
func (r *Relay) getOrCreateUbc(addr *net.UDPAddr) (*udpBufferCh, error) {
	ubc, found := r.udpCache[addr.String()]
	if !found {
		rc, err := net.Dial("udp", r.RemoteUDPAddr)
		if err != nil {
			return nil, err
		}
		ubc := newudpBufferCh(rc)
		r.udpCache[addr.String()] = ubc
		return ubc, nil
	}
	return ubc, nil
}
