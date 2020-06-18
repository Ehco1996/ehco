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
	transport(c, rc)
	rc.Close()
	return nil
}

func (r *Relay) handleOneUDPConn(addr string, ubc *udpBufferCh) {
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	rc, err := net.Dial("udp", r.RemoteUDPAddr)
	if err != nil {
		log.Println(err)
	}

	defer func() {
		rc.Close()
		close(ubc.Ch)
		delete(r.udpCache, addr)
	}()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		buf := outboundBufferPool.Get().([]byte)
		for {
			i, err := rc.Read(buf)
			if err != nil {
				log.Println(err, 1)
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
		outboundBufferPool.Put(buf)
		wg.Done()
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
	wg.Wait()
}
