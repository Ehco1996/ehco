package conn

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Ehco1996/ehco/pkg/buffer"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
)

// wsConn represents a WebSocket connection to relay(io.Copy)
type wsConn struct {
	conn     net.Conn
	isServer bool
	buf      []byte
}

func NewWSConn(conn net.Conn, isServer bool) *wsConn {
	return &wsConn{conn: conn, isServer: isServer, buf: buffer.BufferPool.Get()}
}

func (c *wsConn) Read(b []byte) (n int, err error) {
	header, err := ws.ReadHeader(c.conn)
	if err != nil {
		return 0, err
	}
	if header.Length > int64(cap(c.buf)) {
		zap.S().Warnf("ws payload size:%d is larger than buffer size:%d", header.Length, cap(c.buf))
		return 0, fmt.Errorf("buffer size:%d too small to transport ws payload size:%d", len(b), header.Length)
	}
	payload := c.buf[:header.Length]
	_, err = io.ReadFull(c.conn, payload)
	if err != nil {
		return 0, err
	}
	if header.Masked {
		ws.Cipher(payload, header.Mask, 0)
	}
	if len(payload) > len(b) {
		return 0, fmt.Errorf("buffer size:%d too small to transport ws payload size:%d", len(b), len(payload))
	}
	copy(b, payload)
	return len(payload), nil
}

func (c *wsConn) Write(b []byte) (n int, err error) {
	if c.isServer {
		err = wsutil.WriteServerBinary(c.conn, b)
	} else {
		err = wsutil.WriteClientBinary(c.conn, b)
	}
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *wsConn) Close() error {
	defer buffer.BufferPool.Put(c.buf)
	return c.conn.Close()
}

func (c *wsConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *wsConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *wsConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
