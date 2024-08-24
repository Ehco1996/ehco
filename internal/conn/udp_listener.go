//nolint:errcheck
package conn

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/buffer"
)

var _ net.Conn = &uc{}

type uc struct {
	conn *net.UDPConn
	addr *net.UDPAddr

	msgCh chan []byte

	lastActivity atomic.Value

	listener *UDPListener
}

func (c *uc) Read(b []byte) (int, error) {
	select {
	case msg := <-c.msgCh:
		n := copy(b, msg)
		c.lastActivity.Store(time.Now())
		return n, nil
	default:
		if time.Since(c.lastActivity.Load().(time.Time)) > c.listener.cfg.Options.IdleTimeout {
			return 0, io.EOF
		}
		return 0, nil
	}
}

func (c *uc) Write(b []byte) (int, error) {
	n, err := c.conn.WriteToUDP(b, c.addr)
	c.lastActivity.Store(time.Now())
	return n, err
}

func (c *uc) Close() error {
	c.listener.connsMu.Lock()
	delete(c.listener.conns, c.addr.String())
	c.listener.connsMu.Unlock()
	close(c.msgCh)
	return nil
}

func (c *uc) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *uc) RemoteAddr() net.Addr {
	return c.addr
}

func (c *uc) SetDeadline(t time.Time) error {
	return nil
}

func (c *uc) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *uc) SetWriteDeadline(t time.Time) error {
	return nil
}

type UDPListener struct {
	cfg        *conf.Config
	listenAddr *net.UDPAddr
	listenConn *net.UDPConn

	conns   map[string]*uc
	connsMu sync.RWMutex
	connCh  chan *uc
	msgCh   chan []byte
	errCh   chan error

	ctx    context.Context
	cancel context.CancelFunc

	closed atomic.Bool
}

func NewUDPListener(ctx context.Context, cfg *conf.Config) (*UDPListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", cfg.Listen)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	l := &UDPListener{
		cfg:        cfg,
		listenConn: conn,
		listenAddr: udpAddr,

		conns:  make(map[string]*uc),
		connCh: make(chan *uc),
		msgCh:  make(chan []byte),
		errCh:  make(chan error),
		ctx:    ctx,
		cancel: cancel,
	}

	go l.listen()

	return l, nil
}

func (l *UDPListener) listen() {
	defer l.listenConn.Close()
	for {
		if l.closed.Load() {
			return
		}

		buf := buffer.UDPBufferPool.Get()
		n, addr, err := l.listenConn.ReadFromUDP(buf)
		if err != nil {
			if !l.closed.Load() {
				select {
				case l.errCh <- err:
				default:
				}
			}
			buffer.UDPBufferPool.Put(buf)
			continue
		}

		l.connsMu.RLock()
		udpConn, exists := l.conns[addr.String()]
		l.connsMu.RUnlock()
		if !exists {
			l.connsMu.Lock()
			udpConn = &uc{
				conn:         l.listenConn,
				addr:         addr,
				listener:     l,
				msgCh:        make(chan []byte, 10),
				lastActivity: atomic.Value{},
			}
			udpConn.lastActivity.Store(time.Now())
			l.conns[addr.String()] = udpConn
			l.connCh <- udpConn
			l.connsMu.Unlock()
		}

		select {
		case udpConn.msgCh <- buf[:n]:
		default:
			buffer.UDPBufferPool.Put(buf)
		}
	}
}

func (l *UDPListener) Accept() (*uc, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case err := <-l.errCh:
		return nil, err
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	}
}

func (l *UDPListener) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	l.cancel()
	l.closed.Store(true)
	return l.listenConn.Close()
}
