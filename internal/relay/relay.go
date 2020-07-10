package relay

import (
	"io"
	"net"
	"time"
)

const (
	MaxMWSSStreamCnt = 10
	DialTimeOut      = 10 * time.Second

	Listen_RAW  = "raw"
	Listen_WS   = "ws"
	Listen_WSS  = "wss"
	Listen_MWSS = "mwss"

	Transport_RAW  = "raw"
	Transport_WS   = "ws"
	Transport_WSS  = "wss"
	Transport_MWSS = "mwss"
)

type Relay struct {
	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr

	RemoteTCPAddr string
	RemoteUDPAddr string

	ListenType    string
	TransportType string

	udpCache map[string]*udpBufferCh
	mwssTSP  *mwssTransporter

	// may not init
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

		udpCache: make(map[string](*udpBufferCh)),
		mwssTSP:  NewMWSSTransporter(),
	}

	return r, nil
}

func (r *Relay) ListenAndServe() error {
	errChan := make(chan error)
	Logger.Infof("start relay AT: %s Over: %s TO: %s Through %s",
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
			errChan <- r.RunLocalWSServer()
		}()
	} else if r.ListenType == Listen_WSS {
		go func() {
			errChan <- r.RunLocalWSSServer()
		}()
	} else if r.ListenType == Listen_MWSS {
		go func() {
			errChan <- r.RunLocalMWSSServer()
		}()
	} else {
		Logger.Fatalf("unknown listen type: %s ", r.ListenType)
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
		if err != nil {
			Logger.Infof("accept tcp con error: %s", err)
			return err
		}
		switch r.TransportType {
		case Transport_RAW:
			go func(c *net.TCPConn) {
				defer c.Close()
				if err := r.handleTCPConn(c); err != nil {
					Logger.Infof("handleTCPConn err %s", err)
				}
			}(c)
		case Transport_WS:
			go func(c *net.TCPConn) {
				// need close conn in handleTcpOverWs
				if err := r.handleTcpOverWs(c); err != nil && err != io.EOF {
					Logger.Infof("handleTcpOverWs err %s", err)
				}
			}(c)
		case Transport_WSS:
			go func(c *net.TCPConn) {
				// need close conn in handleTcpOverWss
				if err := r.handleTcpOverWss(c); err != nil && err != io.EOF {
					Logger.Infof("handleTcpOverWss err %s", err)
				}
			}(c)
		case Transport_MWSS:
			go func(c *net.TCPConn) {
				if err := r.handleTcpOverMWSS(c); err != nil && err != io.EOF {
					Logger.Infof("handleTcpOverMWSS err %s", err)
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
	buf := inboundBufferPool.Get().([]byte)
	defer inboundBufferPool.Put(buf)
	for {
		n, addr, err := r.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		ubc, err := r.getOrCreateUbc(addr)
		if err != nil {
			return err
		}
		ubc.Ch <- buf[0:n]
		if !ubc.Handled {
			ubc.Handled = true
			Logger.Infof("handle udp con from %s over: %s", addr, r.TransportType)
			switch r.TransportType {
			case Transport_WSS:
				go r.handleUdpOverWss(addr.String(), ubc)
			case Transport_RAW:
				go r.handleOneUDPConn(addr.String(), ubc)
			}
		}
	}
}

// NOTE not thread safe
func (r *Relay) getOrCreateUbc(addr *net.UDPAddr) (*udpBufferCh, error) {
	ubc, found := r.udpCache[addr.String()]
	if !found {
		ubc := newudpBufferCh()
		r.udpCache[addr.String()] = ubc
		return ubc, nil
	}
	return ubc, nil
}
