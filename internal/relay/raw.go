package relay

import (
	"bufio"
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

func (r *Relay) handleUDP(addr *net.UDPAddr, b []byte) error {
	rc, err := net.Dial("udp", r.RemoteUDPAddr)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := r.keepAliveAndSetNextTimeout(rc); err != nil {
		return err
	}

	if _, err := rc.Write(b); err != nil {
		return err
	}
	var buf [1024 * 2]byte
	i, err := bufio.NewReader(rc).Read(buf[:])
	if err != nil {
		return err
	}
	if _, err := r.UDPConn.WriteToUDP(buf[0:i], addr); err != nil {
		return err
	}

	return nil
}
