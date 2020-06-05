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
	uaddr, _ := net.ResolveUDPAddr("udp", addr)

	defer func() {
		log.Println("defer")
		rc.Close()
		close(ubc.Ch)
		delete(r.udpCache, addr)
	}()

	go func() {
		buf := outboundBufferPool.Get().([]byte)
		for {
			i, err := rc.Read(buf)
			if err != nil {
				log.Println(err, 1)
				break
			}
			if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
				log.Println(err, 2)
				break
			}
			if _, err := r.UDPConn.WriteToUDP(buf[0:i], uaddr); err != nil {
				log.Println(err, 3)
				break

			}
		}
	}()

	for b := range ubc.Ch {
		if _, err := rc.Write(b); err != nil {
			log.Println(err, 4)
			break
		}
		if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
			log.Println(err, 5)
			break
		}
	}
}
