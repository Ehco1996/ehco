package relay

import (
	"bufio"
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
		// TODO clear ubc
	}()
	log.Println("handled", addr)
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	for {
		b := <-ubc.Ch
		log.Println("receive b", len(b))
		if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
			log.Println(err)
		}
		if _, err := rc.Write(b); err != nil {
			log.Println(err)
		}
		buf := make([]byte, 1024*2)
		i, err := bufio.NewReader(rc).Read(buf)
		if err != nil {
			log.Println(err)
		}
		if _, err := r.UDPConn.WriteToUDP(buf[0:i], uaddr); err != nil {
			log.Println(err)
		}
	}
}
