package relay

import (
	"net"
	"sync"
)

func (r *Relay) handleTCPConn(c *net.TCPConn) error {
	defer c.Close()

	addr, node := r.PickTcpRemote()
	if node != nil {
		defer r.LBRemotes.DeferPick(node)
	}

	rc, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := transport(c, rc); err != nil {
		return err
	}
	return nil
}

func (r *Relay) handleOneUDPConn(uaddr *net.UDPAddr, ubc *udpBufferCh) {
	rc, err := net.Dial("udp", r.RemoteUDPAddr)
	if err != nil {
		Logger.Info(err)
		return
	}

	defer func() {
		rc.Close()
		close(ubc.Ch)
		delete(r.udpCache, uaddr.String())
	}()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		buf := outboundBufferPool.Get().([]byte)
		for {
			i, err := rc.Read(buf)
			if err != nil {
				Logger.Info(err, 1)
				break
			}
			if _, err := r.UDPConn.WriteToUDP(buf[0:i], uaddr); err != nil {
				Logger.Info(err)
				break
			}
		}
		outboundBufferPool.Put(buf)
		wg.Done()
	}()

	for b := range ubc.Ch {
		if _, err := rc.Write(b); err != nil {
			Logger.Info(err)
			break
		}
	}
	wg.Wait()
}
