package relay

import (
	"log"
	"net"
	"sync"
)

func (r *Relay) handleTCPConn(c *net.TCPConn) error {
	rc, err := net.Dial("tcp", r.RemoteTCPAddr)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go doCopy(rc, c, inboundBufferPool, &wg)
	go doCopy(c, rc, outboundBufferPool, &wg)
	wg.Wait()
	return nil
}

func (r *Relay) handleOneUDPConn(addr string, ubc *udpBufferCh) {
	rc := ubc.Conn
	defer func() {
		rc.Close()
		// close(ubc.Ch)
		// TODO clear ubc
	}()
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	go func() {
		var buf [1024 * 2]byte
		for {
			i, err := rc.Read(buf[:])
			if err != nil {
				log.Println(err)
				break
			}
			if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
				log.Println(err)
				break
			}
			if _, err := r.UDPConn.WriteToUDP(buf[0:i], uaddr); err != nil {
				log.Println(err)
				break
			}
		}
	}()

	for b := range ubc.Ch {
		if _, err := rc.Write(b); err != nil {
			log.Println(err)
			break
		}
		if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
			log.Println(err)
			break
		}
	}
}
